# Bank Account API

A small HTTP API for bank accounts: create an account, deposit money, and transfer money **atomically** between two accounts. Built as a greenfield, production-minded service, timeboxed to ~4 hours. Data consistency under concurrent transfers is the primary concern.

- **Language / stack:** Go 1.26, standard-library `net/http` (1.22+ routing), GORM over PostgreSQL 16, goose migrations.
- **Money:** stored as integer **minor units** (cents) — never floats. Single currency (EUR assumed).
- **Design spec:** [`docs/superpowers/specs/2026-08-19-bank-account-api-design.md`](docs/superpowers/specs/2026-08-19-bank-account-api-design.md).

## Quick start

Requires Docker (and Docker Compose v2).

```bash
docker compose up --build          # boots Postgres + the API on :8080; migrations run on startup
./scripts/smoke.sh                 # drives the API end-to-end with curl (in another shell)
docker compose down -v             # stop and wipe
```

The API is then on `http://localhost:8080`.

## Build, test, run

```bash
make build     # compile a static binary to bin/api
make test      # go test ./... -race  (needs Docker: integration tests use testcontainers)
make run       # run locally (expects a reachable Postgres via DATABASE_URL)
make up        # docker compose up --build -d
make down      # docker compose down -v
make smoke     # run scripts/smoke.sh against a running API
```

**Configuration** (environment only — no hardcoded paths):

| Variable | Default | Purpose |
|---|---|---|
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/bank?sslmode=disable` | Postgres DSN |
| `PORT` | `8080` | HTTP listen port |

Under `docker compose`, `DATABASE_URL` points at the `db` service automatically.

## API

All amounts are integer minor units (cents). All errors share one envelope:

```json
{ "error": { "code": "insufficient_funds", "message": "insufficient funds" } }
```

| Method | Path | Body | Success |
|---|---|---|---|
| `POST` | `/accounts` | `{"initial_balance": 1000}` (optional, ≥ 0) | `201` |
| `GET` | `/accounts/{id}` | — | `200` |
| `POST` | `/accounts/{id}/deposits` | `{"amount": 500}` (> 0) | `200` |
| `POST` | `/transfers` | `{"from_account_id","to_account_id","amount"}` | `200` |
| `GET` | `/healthz` | — | `200` |

**Status codes:** `201` create · `200` reads/mutations · `400` malformed body or bad UUID (`invalid_request`) · `404` unknown account (`account_not_found`) · `422` business-rule violation (`insufficient_funds`, `invalid_amount`, `self_transfer`) · `500` unexpected (`internal_error`).

### Examples

```bash
# create two accounts
A=$(curl -fsS -X POST localhost:8080/accounts -d '{"initial_balance":1000}' | jq -r .id)
B=$(curl -fsS -X POST localhost:8080/accounts -d '{"initial_balance":0}'    | jq -r .id)

# deposit, then transfer
curl -fsS -X POST localhost:8080/accounts/$A/deposits -d '{"amount":500}'
curl -fsS -X POST localhost:8080/transfers \
  -d "{\"from_account_id\":\"$A\",\"to_account_id\":\"$B\",\"amount\":600}"

# read balance
curl -fsS localhost:8080/accounts/$B
```

A **Postman collection** (self-verifying, chained requests) is at
[`postman/bank-account-api.postman_collection.json`](postman/bank-account-api.postman_collection.json) — import it into Postman and run the collection, or `newman run postman/bank-account-api.postman_collection.json`.

## How consistency is guaranteed

A transfer runs inside a **single database transaction** that locks both account rows with `SELECT ... FOR UPDATE`, always in a **deterministic order (ascending id)**. Locking in the same global order is what makes two opposing concurrent transfers (A→B and B→A) deadlock-free. The source-funds check and both balance updates happen under that lock, so a transfer either commits fully or rolls back — no partial or intermediate state is ever visible.

Defenses, in layers:
- **Application:** the service rejects non-positive amounts and self-transfers up front; the repository re-checks the same invariants at the sink (see trade-offs).
- **Database:** a `CHECK (balance >= 0)` constraint means even a logic bug cannot persist a negative balance.

The concurrency contract is proven by a test that fires 1,000 randomized concurrent transfers and asserts total money is conserved and no balance goes negative (`internal/bank/concurrency_test.go`), plus an opposing-transfer deadlock check — both run clean under `-race`.

## Testing approach

- **Unit** — service-layer validation (`internal/bank/account_service_test.go`), no database, fast.
- **Integration** — the repository against a **real Postgres** via [testcontainers](https://golang.testcontainers.org/): CRUD, transfers, every error path.
- **Concurrency** — money-conservation and deadlock-freedom under load, run with `-race`.
- **End-to-end** — the fully wired HTTP stack against a real Postgres (`internal/api/integration_test.go`).
- **Manual** — `scripts/smoke.sh` (curl) and the Postman collection.

Integration tests require Docker. There is no `-short` gate, so `go test ./...` boots Postgres.

## Project structure

```
cmd/api/                 # main: config -> db -> migrations -> router -> server (graceful shutdown)
internal/config/         # env configuration
internal/bank/           # domain: Account, errors, AccountRepository (interface + GORM impl), AccountService
internal/api/            # net/http router, handlers, DTOs, error mapping, middleware
migrations/              # goose SQL migrations + embedded runner
postman/                 # Postman collection
scripts/smoke.sh         # curl end-to-end demo
```

## Functional requirements

Defining these is part of the exercise; the full list is in the [design spec](docs/superpowers/specs/2026-08-19-bank-account-api-design.md). In short: create accounts (optional initial balance, never negative), deposit positive amounts, transfer positive amounts atomically with sufficient-funds enforcement, conserve money under concurrency, and return consistent JSON errors.

## Design decisions & trade-offs

Deliberate choices, and what each one costs:

- **Integer minor units, not floats.** Money is stored as whole cents (int64), so arithmetic is exact — floats can't represent values like 0.10 precisely (0.1 + 0.2 ≠ 0.3), and those errors accumulate. Amounts cross the API as integers (500 = €5.00). Single currency, so no currency column yet.
- **Single currency.** Corner cut. No `currency` column. I'd suggest the `currency` field with cross-currency rejection to be the first extension.
- **Balance-only, no ledger.** Balances are mutated in place rather than derived from an append-only ledger. Simpler and faster to build, but there is **no audit trail or historical reconstruction**. A production banking system would write immutable transaction (ideally double-entry) rows in the same transaction as the balance change. This is the **top future improvement**.
- **No idempotency keys.** A client that retries after a network timeout could double-deposit or double-transfer. Production money movement needs an `Idempotency-Key` (store key → result, replay the prior result). Deliberately deferred. It is a known gap - another corner cut.
- **Pessimistic, ordered locking.** `FOR UPDATE` on both rows in ascending-id order is simple, correct, and deadlock-free. Optimistic concurrency (a version column + retry) can perform better under low contention but adds retry logic. Chosen against for clarity and correctness-first.
- **Defense-in-depth validation.** The service validates (clean early rejection) *and* the repository re-validates self-transfer and non-positive amounts at the sink. The validation duplication is intentional: the repository is the ultimate sink for balance mutations, and a self-transfer would otherwise be a genuine fund-duplication bug (two `UPDATE`s to the same row; the `CHECK` constraint does not catch it).
- **GORM.** GORM was chosen to have readable and maintainabile code on the common CRUD paths (models map to tables, no manual row scanning). The cost is that it abstracts the emitted SQL, which is mild friction on the one path where consistency is paramount; mitigated by using explicit `clause.Locking` on the transfer and keeping that path small. Raw `pgx`, for example, would be more transparent there but more verbose everywhere else.
- **`422` for insufficient funds.** The request is well-formed but semantically unprocessable. `409 Conflict` (a state conflict) is a defensible alternative.
- **Repository co-located with its interface** in the `bank` package. Flatter and simpler for a single-database app; the trade-off is that the domain package imports GORM (persistence leaks into the domain). A stricter hexagonal layout would put the implementation in its own package behind the port.
- **Testcontainers for integration tests.** Real Postgres means real locking and real SQL, at the cost of requiring Docker. The container setup is currently duplicated between the `bank` package's `TestMain` and the API end-to-end test's `newStack`, so `go test ./...` boots two containers — left as a timebox trade-off; a shared `internal/testsupport` helper (one container) would remove it.

## Security & multi-tenancy (documented, not implemented)

Authentication and authorization are out of scope, so the API currently treats all accounts as globally accessible. In production, account access must be tenant/owner-scoped, and beyond app-layer authorization, **Postgres Row-Level Security (RLS)** is the defense-in-depth layer: policies on `accounts` restricting `SELECT`/`UPDATE` to rows the caller owns, so even an authorization bug cannot leak or mutate another customer's account. Two real complications worth discussing: setting the per-request principal as `SET LOCAL` *inside each transaction* (so it never leaks across GORM's pooled connections), and the fact that a transfer crosses ownership boundaries (a naive "only your rows" policy blocks the credit — resolved via a `SECURITY DEFINER` function or a trusted service role). RLS is the intended production direction; it is **not implemented** here.

## Known limitations & next steps

Append-only ledger / double-entry accounting · idempotency keys · authentication + authorization + RLS · multi-currency · account listing/pagination · richer observability (metrics/tracing) · rate limiting · a shared testcontainers helper to de-duplicate integration setup.
