# Bank Account API — Design

**Date:** 2026-08-19
**Status:** Approved design, pre-implementation
**Context:** SumUp backend take-home. Greenfield, production-minded, timeboxed to ~4 hours.

## Build priorities (in order)

1. **Data consistency** — correct, atomic money movement under concurrency. This is the primary evaluation axis and where effort is concentrated.
2. **Documentation & trade-offs** — clear README/spec, and honest, intentional write-ups of the decisions made. *(Trade-off text is reviewed with the author before finalizing.)*

Everything else (feature breadth, edge cases) is deliberately kept minimal per the "resist over-engineering / timebox" guidance.

## Overview

A small HTTP API for bank accounts supporting three operations: create an account, deposit money, and transfer money atomically between two accounts. Money is stored as integer minor units (cents), single currency (EUR assumed). Persistence is PostgreSQL; correctness under concurrent transfers is the central concern.

## Functional requirements

We define these ourselves (defining them is part of the exercise).

**Accounts**
- **FR1** — Create an account with an optional initial balance (default `0`). Balance can never be negative.
- **FR2** — Fetch an account's current balance by id.

**Money movement**
- **FR3** — Deposit a positive amount into an existing account.
- **FR4** — Transfer a positive amount from one account to another **atomically**: both balances change or neither does.
- **FR5** — A transfer is rejected when the source has insufficient funds; no partial or intermediate state is ever observable.
- **FR6** — Concurrent transfers never create or destroy money (the sum of all balances is invariant) and never drive any balance negative.

**Cross-cutting**
- **FR7** — All amounts are integer minor units (cents), single currency. Non-positive amounts are rejected.
- **FR8** — Every error returns a consistent JSON body with a correct HTTP status code.

### Out of scope (deliberately deferred, documented as future work)

Authentication/authorization, ledger/transaction history, idempotency keys, multi-currency/FX, pagination, account deletion/closing, interest, overdraft limits, Row-Level Security (see Security section).

## Non-functional requirements

Scaled to a timeboxed exercise; the first four map directly to the assignment's "Requirements" table. Each is marked **in scope** (built now) or **best-effort / documented** (touched lightly, discussed as future work).

- **NFR1 — Correctness & consistency (in scope, priority #1).** Money is conserved across all operations (sum of balances is invariant under transfers), transfers are atomic, and no balance can go negative. Enforced by a single locking transaction plus a `CHECK (balance >= 0)` DB constraint.
- **NFR2 — Concurrency safety (in scope).** The API is safe under concurrent requests. Competing transfers on the same accounts are serialized by deterministic, ordered row locks; the design is deadlock-free by construction and validated by a dedicated concurrency test.
- **NFR3 — Runnable (in scope).** A single documented command starts the full system — `docker compose up` (via `compose.yaml`) or `make run` — including Postgres and automatic migration on startup.
- **NFR4 — Buildable (in scope).** Clear, documented build/dependency steps; a multi-stage `Dockerfile` and `make build` produce the binary with no manual setup.
- **NFR5 — Testable (in scope).** Automated tests (unit + integration + concurrency) runnable via `make test`, plus a `curl` smoke script for manual verification.
- **NFR6 — Portable (in scope).** Runs on any machine with Docker. Configuration is entirely via environment variables; no hardcoded local paths or implicit host dependencies.
- **NFR7 — Observability (best-effort).** Structured request logging with a per-request id and a `GET /healthz` endpoint. Metrics/tracing and audit loggs are noted as future work.
- **NFR8 — Security (best-effort / documented).** Input validation and parameterized queries (no injection). Authentication/authorization and Postgres RLS are out of scope and documented as the production direction (see Security section).
- **NFR9 — Maintainability (in scope).** Small, single-purpose packages with clear boundaries (`config`, `bank`, `postgres`, `api`) that can be understood and tested independently.
- **NFR10 — Graceful lifecycle (best-effort).** The server shuts down gracefully on `SIGINT`/`SIGTERM`, draining in-flight requests and closing the DB pool.

**Explicit non-goals for this exercise:** horizontal scalability, high-availability/failover, load/performance targets, and multi-region concerns. Single-node Postgres is assumed; scaling strategies are interview discussion points, not deliverables.

## API surface

| Method | Path | Purpose | Success |
|---|---|---|---|
| `POST` | `/accounts` | Create account | `201` |
| `GET` | `/accounts/{id}` | Get account + balance | `200` |
| `POST` | `/accounts/{id}/deposits` | Deposit into account | `200` |
| `POST` | `/transfers` | Transfer between two accounts | `200` |
| `GET` | `/healthz` | Liveness/readiness | `200` |

**Payloads** (all amounts are integer cents):

- **Create** — `{ "initial_balance": 1000 }` (optional, default `0`, must be `>= 0`)
  → `201 { "id": "<uuid>", "balance": 1000, "created_at": "..." }`
- **Deposit** — `{ "amount": 500 }` (must be `> 0`)
  → `200 { "id": "<uuid>", "balance": 1500 }`
- **Transfer** — `{ "from_account_id": "<uuid>", "to_account_id": "<uuid>", "amount": 250 }`
  → `200 { "from_account_id", "to_account_id", "amount", "from_balance", "to_balance" }`

**Design choices**
- **Server-generated UUID ids** — no enumeration, no client coordination.
- **Deposit is a sub-resource** of an account (`/accounts/{id}/deposits`); **transfer is a top-level resource** (`/transfers`) because it spans two accounts.
- Alternative considered (documented, not chosen): a single `POST /accounts/{id}/transactions` endpoint with a `type` discriminator. Rejected for clarity — distinct operations read better as distinct routes at this size.

## Status codes & error model

- `201` create · `200` reads and successful mutations · `400` malformed JSON / wrong types · `404` unknown account · `422` business-rule violation (insufficient funds, non-positive amount, self-transfer) · `500` unexpected.
- **Insufficient funds → `422`**. `409 Conflict` is a defensible alternative (treating it as a state conflict); we choose `422` because the request is well-formed but semantically unprocessable. Documented as a discussion point.
- Uniform error body with a stable, machine-readable `code`:

```json
{ "error": { "code": "insufficient_funds", "message": "source account has insufficient funds" } }
```

Error codes (initial set): `invalid_request`, `account_not_found`, `insufficient_funds`, `invalid_amount`, `self_transfer`, `internal_error`.

## Data model & concurrency (core)

Single table (balance-only; see the ledger trade-off below):

```sql
CREATE TABLE accounts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    balance    BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**Transfer = one DB transaction with pessimistic, ordered row locking:**

1. `BEGIN`
2. `SELECT ... FROM accounts WHERE id IN ($from, $to) ORDER BY id FOR UPDATE` — locking **both rows in a deterministic order (ascending id)** is what prevents deadlocks between two opposing concurrent transfers (A→B and B→A).
3. Verify both accounts exist; verify `source.balance >= amount`.
4. `UPDATE` source `-= amount`, `UPDATE` destination `+= amount`.
5. `COMMIT`.

Expressed via GORM:

```go
err := db.Transaction(func(tx *gorm.DB) error {
    var accts []Account
    // ids is sorted ascending → deterministic lock order → deadlock-free
    if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
        Where("id IN ?", ids).
        Order("id").
        Find(&accts); err != nil {
        return err
    }
    // verify both exist, check source funds, update both balances
    // ...
    return nil
})
```

**Defense in depth:** the `CHECK (balance >= 0)` constraint means even an application-logic bug cannot persist a negative balance — Postgres rejects the write. The application still checks funds explicitly to return a clean `422` rather than relying on a constraint violation for control flow.

**Concurrency correctness argument:** all reads and writes for a transfer occur inside a single serialized-by-locks transaction. Because every transfer locks its two rows in the same global order, no two transfers can hold locks in a cycle, so there is no deadlock. Money conservation follows from atomicity: the two balance updates commit together or not at all.

## Stack & project layout

- **Go 1.26**
- **Routing:** standard library `net/http` (1.22+ method + path routing, `r.PathValue`). Middleware (request-id, panic recovery, logging) hand-written as `func(http.Handler) http.Handler`. No third-party router.
- **DB access:** GORM (`gorm.io/gorm`, `gorm.io/driver/postgres`).
- **Migrations:** goose (`github.com/pressly/goose/v3`), SQL migrations embedded via `embed.FS`, applied on startup; Makefile targets for `create/up/down`.
- **Ids:** `google/uuid`.
- **Config:** environment variables only (no hardcoded paths).
- **Test-only:** `testcontainers-go`, `testify`.

```
cmd/api/main.go            # wiring: config -> db -> migrations -> router -> server
internal/
  config/                  # env parsing (DATABASE_URL, PORT)
  bank/                    # domain types + business rules (validation, domain errors)
  postgres/                # GORM store: accounts + transfer tx, ordered locking
  api/                     # net/http router, handlers, JSON encode/decode, error mapping, middleware
migrations/                # goose SQL migrations (embedded)
scripts/smoke.sh           # curl-based end-to-end demo
compose.yaml               # app + postgres
Dockerfile                 # multi-stage, static binary
Makefile                   # run, test, build, migrate
README.md
```

## Migrations & schema ownership

**goose owns the schema; GORM does not.** GORM `AutoMigrate` is disabled entirely — GORM models are query mappings over goose-managed tables. This removes any ambiguity about which tool controls DDL and keeps the `CHECK` constraint and defaults under explicit, reviewable control. Migrations are embedded and run automatically on startup so `compose up` is fully self-contained.

## Configuration & portability

- All configuration via environment (`DATABASE_URL`, `PORT`, etc.). No machine-specific paths or implicit dependencies.
- `compose.yaml` brings up Postgres + the API and runs migrations automatically.
- `make run` / `docker compose up` start the service; `make test` runs the suite.
- Multi-stage `Dockerfile` produces a small static binary.

## Testing approach

- **Unit** — business-rule validation in `bank` (non-positive amount, self-transfer, insufficient funds) without a database.
- **Integration** — real Postgres via **testcontainers-go**, exercised end-to-end through the HTTP handlers: create/deposit/transfer happy paths and every error path (404, 422, malformed).
- **Concurrency test (the key one)** — many goroutines issue concurrent transfers across a ring of accounts; afterward assert (a) the total balance across all accounts is unchanged and (b) no balance is negative. Directly demonstrates FR6.
- **Manual** — `scripts/smoke.sh` (curl) plus copy-paste examples in the README.

Testcontainers is chosen over a compose-based integration DB so `go test ./...` is self-contained (requires Docker available on the test machine — documented).

## Security & multi-tenancy (documented, not implemented)

Authentication and authorization are out of scope, so the current API treats all accounts as globally accessible. In production, account access must be tenant/owner-scoped. Beyond app-layer authorization, **Postgres Row-Level Security (RLS)** is the defense-in-depth layer: policies on `accounts` restrict `SELECT`/`UPDATE` to rows the calling principal owns, so even an app-layer authorization bug cannot leak or mutate another customer's account.

Two complications worth discussing rather than hand-waving:

1. **Session identity with a connection pool.** RLS policies read a per-request identity (e.g. `current_setting('app.current_principal')`). With GORM's pooled connections this must be set as `SET LOCAL ...` *inside each transaction* so it is scoped to that transaction and never leaks across pooled connections.
2. **Transfers cross ownership boundaries.** A transfer debits the caller's account but credits *someone else's*. A naive "you may only touch rows you own" policy would block the credit. The clean resolution is to keep RLS for direct account reads/writes and perform money movement through a trusted, auditable path (a `SECURITY DEFINER` function or a dedicated service role) that cannot be invoked to bypass ownership on the debit side.

RLS is mentioned as the production direction; it is **not implemented** in this exercise.

## Trade-offs & decisions

*(This section is the primary material for the interview discussion. It is reviewed with the author before finalizing.)*

- **Balance-only, no ledger.** We mutate `accounts.balance` directly rather than deriving balances from an append-only ledger. Simpler and faster to build, but we lose an audit trail and the ability to reconstruct history. A production banking system would record immutable `transactions` (ideally double-entry) rows in the same transaction as the balance change. Deliberately deferred to respect the timebox; called out as the top future improvement.
- **No idempotency keys.** Money-moving endpoints are not idempotent, so a client retry after a network timeout could double-deposit or double-transfer. Production-grade money movement needs an `Idempotency-Key` (store key → result, replay prior result). Deferred and documented as a known gap.
- **GORM vs raw `pgx`.** GORM improves readability and maintainability on the common paths: models map directly to tables, there's no manual row scanning or SQL string-building, and query code reads declaratively — less boilerplate to write and maintain. The cost is that it abstracts the emitted SQL, which is mild friction on the one path where data consistency is paramount. Mitigation: the critical transfer path uses explicit `clause.Locking` and the generated SQL is verified via GORM's logger. Raw `pgx` would be more transparent on the locking path but more verbose everywhere else — the honest counterpoint.
- **`422` vs `409` for insufficient funds.** Chose `422` (well-formed but unprocessable). `409` (state conflict) is defensible.
- **Pessimistic ordered locks vs optimistic concurrency.** Pessimistic ordered `FOR UPDATE` is simple and deadlock-free and explains cleanly. Optimistic (version + retry) can perform better under low contention but adds retry logic. Chose pessimistic for clarity and correctness first.
- **Single currency.** No currency column or FX. A `currency` column with cross-currency rejection would be the first extension.
- **testcontainers requires Docker** on the test machine — accepted for self-contained tests.
- **goose owns schema (no GORM AutoMigrate)** — one source of truth for DDL, at the cost of writing SQL migrations by hand (a feature, not a bug, for reviewability).

## Future improvements

Append-only ledger / double-entry accounting · idempotency keys · authentication + authorization + **RLS** · multi-currency · account listing/pagination · structured audit logging · metrics/observability · rate limiting · optimistic-concurrency option under high contention.
