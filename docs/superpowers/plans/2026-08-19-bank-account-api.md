# Bank Account API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a production-minded HTTP API to create accounts, deposit money, and transfer money atomically between accounts, backed by PostgreSQL.

**Architecture:** `config` (env), `bank` (domain types, sentinel errors, the `AccountRepository` interface co-located with its GORM implementation `Repository`, and the `AccountService` that validates then delegates), `api` (stdlib `net/http` router, handlers, error mapping, middleware), and a top-level `migrations` package (embedded goose SQL applied via `Up`). `main` wires them and handles graceful shutdown. Correctness under concurrency lives in a single locking DB transaction; the `AccountRepository` interface keeps the domain service unit-testable with a fake.

**Tech Stack:** Go 1.26, stdlib `net/http` (1.22+ routing), GORM + `gorm.io/driver/postgres`, goose migrations, `google/uuid`, PostgreSQL 16. Tests: `testify` + `testcontainers-go`.

## Global Constraints

Every task's requirements implicitly include this section.

- **Go 1.26** — `go 1.26` in `go.mod`; `golang:1.26-alpine` build image.
- **Module path:** `github.com/pavlomaksymov/bank-account-api`.
- **Execution discipline:** strict TDD **red → green → refactor** for every task — write the failing test, watch it fail, write the minimal code to pass, watch it pass, then refactor with tests green. Commit after each task.
- **Comments:** minimum doc blocks, only where necessary. Code must be self-documenting through naming and structure. No restating-the-obvious comments, no ceremonial doc blocks on self-evident functions. A comment earns its place only by explaining *why* something non-obvious is done (e.g. the ordered-lock rationale).
- **Testability:** dependencies injected via constructors; no global state; no `init()` side effects; pure validation kept free of I/O.
- **Routing:** stdlib `net/http` only. No third-party router.
- **DB:** GORM for queries. **goose owns the schema — GORM `AutoMigrate` is never called.**
- **Money:** `int64` minor units (cents). Single currency (EUR assumed). Reject non-positive amounts.
- **Errors:** domain sentinel errors defined in `bank`; HTTP status/code mapping isolated in `api`.
- **Config:** environment variables only. No hardcoded local paths.
- **Insufficient funds → HTTP 422.** Unknown account → 404. Malformed request → 400. Static rule violation → 422.

## File Structure

```
go.mod / go.sum
.gitignore
.dockerignore
Makefile
compose.yaml
Dockerfile
cmd/api/main.go                        # wiring + graceful shutdown
internal/config/config.go              # Load() from env
internal/config/config_test.go
internal/bank/account.go               # Account, TransferResult
internal/bank/errors.go                # sentinel errors
internal/bank/store.go                 # Store interface
internal/bank/service.go               # Service: static validation + delegation
internal/bank/service_test.go          # unit tests with a fake Store
internal/bank/account_repository.go            # accountRow (GORM), toDomain
migrations/migrations.go           # embedded goose runner
migrations/0001_init.sql
internal/bank/account_repository.go             # Store: Create/Get/Deposit/Transfer
internal/bank/account_repository_test.go        # integration (testcontainers) + TestMain
internal/api/dto.go                    # request/response structs
internal/api/errors.go                 # error -> HTTP mapping, JSON helpers
internal/api/handlers.go               # Handler, Service interface
internal/api/middleware.go             # requestID, logger, recoverer
internal/api/router.go                 # NewRouter
internal/api/handlers_test.go          # handler unit tests (fake Service)
internal/api/integration_test.go       # full stack via testcontainers
scripts/smoke.sh                       # curl end-to-end demo
README.md
```

---

## Task 1: Project bootstrap

**Files:**
- Create: `go.mod`, `.gitignore`, `.dockerignore`, `Makefile`
- Test: `internal/config/config_test.go` (placeholder compile target in Task 2; here we only verify the toolchain builds)

**Interfaces:**
- Produces: a compiling Go module at `github.com/pavlomaksymov/bank-account-api` with all runtime + test dependencies resolved.

- [ ] **Step 1: Initialise the module and add dependencies**

```bash
go mod init github.com/pavlomaksymov/bank-account-api
go get gorm.io/gorm gorm.io/driver/postgres github.com/pressly/goose/v3 github.com/google/uuid
go get github.com/stretchr/testify github.com/testcontainers/testcontainers-go github.com/testcontainers/testcontainers-go/modules/postgres
```

- [ ] **Step 2: Pin the Go version**

Ensure `go.mod` begins with:

```
module github.com/pavlomaksymov/bank-account-api

go 1.26
```

- [ ] **Step 3: Add `.gitignore`**

```gitignore
/bin/
/tmp/
*.out
.env
.DS_Store
```

- [ ] **Step 4: Add `.dockerignore`**

```dockerignore
.git
bin
tmp
*.md
docs
scripts
```

- [ ] **Step 5: Add a minimal `Makefile`** (targets are fleshed out in Task 13)

```makefile
.PHONY: build run test tidy
build:
	CGO_ENABLED=0 go build -o bin/api ./cmd/api
test:
	go test ./...
tidy:
	go mod tidy
run:
	go run ./cmd/api
```

- [ ] **Step 6: Verify the module builds**

Run: `go build ./... && go vet ./...`
Expected: no output, exit 0.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum .gitignore .dockerignore Makefile
git commit -m "chore: bootstrap go module and tooling"
```

---

## Task 2: Config loading

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `type Config struct { DatabaseURL string; Port string }` and `func Load() Config`.
  Defaults: `Port="8080"`, `DatabaseURL="postgres://postgres:postgres@localhost:5432/bank?sslmode=disable"`. Env vars `PORT` and `DATABASE_URL` override.

- [ ] **Step 1: Write the failing test**

```go
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")

	cfg := Load()

	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "postgres://postgres:postgres@localhost:5432/bank?sslmode=disable", cfg.DatabaseURL)
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://u:p@db:5432/x?sslmode=disable")

	cfg := Load()

	assert.Equal(t, "9090", cfg.Port)
	assert.Equal(t, "postgres://u:p@db:5432/x?sslmode=disable", cfg.DatabaseURL)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad -v`
Expected: FAIL — `undefined: Load`.

- [ ] **Step 3: Write minimal implementation**

```go
package config

import "os"

type Config struct {
	DatabaseURL string
	Port        string
}

func Load() Config {
	return Config{
		DatabaseURL: getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/bank?sslmode=disable"),
		Port:        getenv("PORT", "8080"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestLoad -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: load configuration from environment"
```

---

## Task 3: Domain types and sentinel errors

**Files:**
- Create: `internal/bank/account.go`, `internal/bank/errors.go`

**Interfaces:**
- Produces:
  - `type Account struct { ID uuid.UUID; Balance int64; CreatedAt time.Time }`
  - `type TransferResult struct { FromAccountID, ToAccountID uuid.UUID; Amount, FromBalance, ToBalance int64 }`
  - sentinel errors: `ErrAccountNotFound`, `ErrInsufficientFunds`, `ErrInvalidAmount`, `ErrNegativeBalance`, `ErrSelfTransfer`.

- [ ] **Step 1: Write the failing test**

```go
package bank

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSentinelErrors_HaveStableMessages(t *testing.T) {
	assert.EqualError(t, ErrAccountNotFound, "account not found")
	assert.EqualError(t, ErrInsufficientFunds, "insufficient funds")
	assert.EqualError(t, ErrInvalidAmount, "amount must be positive")
	assert.EqualError(t, ErrNegativeBalance, "initial balance must not be negative")
	assert.EqualError(t, ErrSelfTransfer, "cannot transfer to the same account")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bank/ -run TestSentinelErrors -v`
Expected: FAIL — undefined error identifiers.

- [ ] **Step 3: Write minimal implementation**

`internal/bank/account.go`:

```go
package bank

import (
	"time"

	"github.com/google/uuid"
)

type Account struct {
	ID        uuid.UUID
	Balance   int64
	CreatedAt time.Time
}

type TransferResult struct {
	FromAccountID uuid.UUID
	ToAccountID   uuid.UUID
	Amount        int64
	FromBalance   int64
	ToBalance     int64
}
```

`internal/bank/errors.go`:

```go
package bank

import "errors"

var (
	ErrAccountNotFound   = errors.New("account not found")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrInvalidAmount     = errors.New("amount must be positive")
	ErrNegativeBalance   = errors.New("initial balance must not be negative")
	ErrSelfTransfer      = errors.New("cannot transfer to the same account")
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bank/ -run TestSentinelErrors -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bank/account.go internal/bank/errors.go
git commit -m "feat: add bank domain types and sentinel errors"
```

---

## Task 4: Service with static validation (fake Store)

**Files:**
- Create: `internal/bank/store.go`, `internal/bank/service.go`
- Test: `internal/bank/service_test.go`

**Interfaces:**
- Produces:
  - `type AccountRepository interface { Create(ctx, initialBalance int64) (Account, error); Get(ctx, id uuid.UUID) (Account, error); Deposit(ctx, id uuid.UUID, amount int64) (Account, error); Transfer(ctx, from, to uuid.UUID, amount int64) (TransferResult, error) }` (all take `context.Context` first).
  - `type AccountService struct{...}`, `func NewAccountService(store Store) *Service`, and methods `Create`, `Get`, `Deposit`, `Transfer` with the same signatures as `Store`.
- Contract: `Service` performs only **static** validation (amount > 0, initial balance ≥ 0, from ≠ to) then delegates. Existence and funds checks belong to the store (they require the lock).

- [ ] **Step 1: Write the failing test**

```go
package bank

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	createCalls   int
	depositCalls  int
	transferCalls int
	account       Account
	transfer      TransferResult
}

func (f *fakeStore) Create(_ context.Context, initialBalance int64) (Account, error) {
	f.createCalls++
	return Account{ID: uuid.New(), Balance: initialBalance}, nil
}
func (f *fakeStore) Get(_ context.Context, id uuid.UUID) (Account, error) {
	return Account{ID: id}, nil
}
func (f *fakeStore) Deposit(_ context.Context, id uuid.UUID, amount int64) (Account, error) {
	f.depositCalls++
	return Account{ID: id, Balance: amount}, nil
}
func (f *fakeStore) Transfer(_ context.Context, from, to uuid.UUID, amount int64) (TransferResult, error) {
	f.transferCalls++
	return TransferResult{FromAccountID: from, ToAccountID: to, Amount: amount}, nil
}

func TestService_Create_RejectsNegativeBalance(t *testing.T) {
	fs := &fakeStore{}
	svc := NewAccountService(fs)

	_, err := svc.Create(context.Background(), -1)

	assert.ErrorIs(t, err, ErrNegativeBalance)
	assert.Equal(t, 0, fs.createCalls, "store must not be called on invalid input")
}

func TestService_Create_AllowsZeroAndPositive(t *testing.T) {
	fs := &fakeStore{}
	svc := NewAccountService(fs)

	_, err := svc.Create(context.Background(), 0)
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, 2, fs.createCalls)
}

func TestService_Deposit_RejectsNonPositive(t *testing.T) {
	fs := &fakeStore{}
	svc := NewAccountService(fs)

	_, err := svc.Deposit(context.Background(), uuid.New(), 0)

	assert.ErrorIs(t, err, ErrInvalidAmount)
	assert.Equal(t, 0, fs.depositCalls)
}

func TestService_Transfer_RejectsNonPositive(t *testing.T) {
	fs := &fakeStore{}
	svc := NewAccountService(fs)

	_, err := svc.Transfer(context.Background(), uuid.New(), uuid.New(), -5)

	assert.ErrorIs(t, err, ErrInvalidAmount)
	assert.Equal(t, 0, fs.transferCalls)
}

func TestService_Transfer_RejectsSelfTransfer(t *testing.T) {
	fs := &fakeStore{}
	svc := NewAccountService(fs)
	id := uuid.New()

	_, err := svc.Transfer(context.Background(), id, id, 10)

	assert.ErrorIs(t, err, ErrSelfTransfer)
	assert.Equal(t, 0, fs.transferCalls)
}

func TestService_Transfer_DelegatesWhenValid(t *testing.T) {
	fs := &fakeStore{}
	svc := NewAccountService(fs)

	_, err := svc.Transfer(context.Background(), uuid.New(), uuid.New(), 10)

	require.NoError(t, err)
	assert.Equal(t, 1, fs.transferCalls)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bank/ -run TestService -v`
Expected: FAIL — `undefined: NewAccountService` / `AccountRepository`.

- [ ] **Step 3: Write minimal implementation**

`internal/bank/store.go`:

```go
package bank

import (
	"context"

	"github.com/google/uuid"
)

type AccountRepository interface {
	Create(ctx context.Context, initialBalance int64) (Account, error)
	Get(ctx context.Context, id uuid.UUID) (Account, error)
	Deposit(ctx context.Context, id uuid.UUID, amount int64) (Account, error)
	Transfer(ctx context.Context, from, to uuid.UUID, amount int64) (TransferResult, error)
}
```

`internal/bank/service.go`:

```go
package bank

import (
	"context"

	"github.com/google/uuid"
)

type AccountService struct {
	store Store
}

func NewAccountService(store Store) *Service {
	return &AccountService{store: store}
}

func (s *AccountService) Create(ctx context.Context, initialBalance int64) (Account, error) {
	if initialBalance < 0 {
		return Account{}, ErrNegativeBalance
	}
	return s.store.Create(ctx, initialBalance)
}

func (s *AccountService) Get(ctx context.Context, id uuid.UUID) (Account, error) {
	return s.store.Get(ctx, id)
}

func (s *AccountService) Deposit(ctx context.Context, id uuid.UUID, amount int64) (Account, error) {
	if amount <= 0 {
		return Account{}, ErrInvalidAmount
	}
	return s.store.Deposit(ctx, id, amount)
}

func (s *AccountService) Transfer(ctx context.Context, from, to uuid.UUID, amount int64) (TransferResult, error) {
	if amount <= 0 {
		return TransferResult{}, ErrInvalidAmount
	}
	if from == to {
		return TransferResult{}, ErrSelfTransfer
	}
	return s.store.Transfer(ctx, from, to, amount)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bank/ -v`
Expected: PASS (all `bank` tests).

- [ ] **Step 5: Refactor check**

Confirm no duplicated validation, names read clearly, no stray comments. Re-run `go test ./internal/bank/`.

- [ ] **Step 6: Commit**

```bash
git add internal/bank/store.go internal/bank/service.go internal/bank/service_test.go
git commit -m "feat: add bank service with static validation"
```

---

## Task 5: Postgres store — migrations, setup, Create, Get

**Files:**
- Create: `internal/bank/account_repository.go`, `migrations/migrations.go`, `migrations/0001_init.sql`, `internal/bank/account_repository.go`, `internal/bank/account_repository_test.go`

**Interfaces:**
- Consumes: `bank.Account`, `bank.ErrAccountNotFound`, `bank.AccountRepository`.
- Produces:
  - `func NewRepository(db *gorm.DB) *Repository` — `*Repository` implements `bank.AccountRepository`.
  - `func Migrate(db *sql.DB) error` — applies embedded goose migrations.
  - test helpers `testRepo *Repository` (package-level, started in `TestMain`) and `truncate(t *testing.T)`.

- [ ] **Step 1: Write the migration**

`migrations/0001_init.sql`:

```sql
-- +goose Up
CREATE TABLE accounts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    balance    BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE accounts;
```

- [ ] **Step 2: Write the migration runner**

`migrations/migrations.go`:

```go
package bank

import (
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Migrate(db *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(db, "migrations")
}
```

- [ ] **Step 3: Write the failing test (with testcontainers harness)**

`internal/bank/account_repository_test.go`:

```go
package bank

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/pavlomaksymov/bank-account-api/internal/bank"
)

var testRepo *Repository

func TestMain(m *testing.M) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("bank"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		panic(err)
	}
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}
	gdb, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		panic(err)
	}
	if err := Migrate(sqlDB); err != nil {
		panic(err)
	}
	testRepo = NewRepository(gdb)

	code := m.Run()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

func truncate(t *testing.T) {
	t.Helper()
	require.NoError(t, testRepo.db.Exec("TRUNCATE accounts").Error)
}

func TestRepository_CreateAndGet(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	created, err := testRepo.Create(ctx, 500)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, created.ID)
	assert.Equal(t, int64(500), created.Balance)
	assert.False(t, created.CreatedAt.IsZero())

	got, err := testRepo.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, int64(500), got.Balance)
}

func TestRepository_Get_NotFound(t *testing.T) {
	truncate(t)

	_, err := testRepo.Get(context.Background(), uuid.New())

	assert.ErrorIs(t, err, bank.ErrAccountNotFound)
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/bank/ -run TestRepository_CreateAndGet -v`
Expected: FAIL — `undefined: NewRepository` / `Repository`. (Docker must be running.)

- [ ] **Step 5: Write minimal implementation**

`internal/bank/account_repository.go`:

```go
package bank

import (
	"time"

	"github.com/google/uuid"

	"github.com/pavlomaksymov/bank-account-api/internal/bank"
)

type accountRow struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	Balance   int64     `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null"`
}

func (accountRow) TableName() string { return "accounts" }

func (r accountRow) toDomain() bank.Account {
	return bank.Account{ID: r.ID, Balance: r.Balance, CreatedAt: r.CreatedAt}
}
```

`internal/bank/account_repository.go` (Create + Get only for now):

```go
package bank

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pavlomaksymov/bank-account-api/internal/bank"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (s *Repository) Create(ctx context.Context, initialBalance int64) (bank.Account, error) {
	row := accountRow{ID: uuid.New(), Balance: initialBalance}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return bank.Account{}, err
	}
	return row.toDomain(), nil
}

func (s *Repository) Get(ctx context.Context, id uuid.UUID) (bank.Account, error) {
	var row accountRow
	err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return bank.Account{}, bank.ErrAccountNotFound
	}
	if err != nil {
		return bank.Account{}, err
	}
	return row.toDomain(), nil
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/bank/ -run 'TestRepository_CreateAndGet|TestRepository_Get_NotFound' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/bank/
git commit -m "feat: add postgres store with migrations, create and get account"
```

---

## Task 6: Postgres Deposit

**Files:**
- Modify: `internal/bank/account_repository.go`
- Test: `internal/bank/account_repository_test.go`

**Interfaces:**
- Produces: `func (s *Repository) Deposit(ctx, id uuid.UUID, amount int64) (bank.Account, error)` — locks the row, adds `amount`, returns the updated account; unknown id → `bank.ErrAccountNotFound`.

- [ ] **Step 1: Write the failing test**

Append to `internal/bank/account_repository_test.go`:

```go
func TestRepository_Deposit_IncreasesBalance(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	acc, err := testRepo.Create(ctx, 100)
	require.NoError(t, err)

	updated, err := testRepo.Deposit(ctx, acc.ID, 250)
	require.NoError(t, err)
	assert.Equal(t, int64(350), updated.Balance)

	got, err := testRepo.Get(ctx, acc.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(350), got.Balance)
}

func TestRepository_Deposit_NotFound(t *testing.T) {
	truncate(t)

	_, err := testRepo.Deposit(context.Background(), uuid.New(), 50)

	assert.ErrorIs(t, err, bank.ErrAccountNotFound)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bank/ -run TestRepository_Deposit -v`
Expected: FAIL — `undefined: (*Repository).Deposit`.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/bank/account_repository.go`:

```go
import "gorm.io/gorm/clause" // add to the import block

func (s *Repository) Deposit(ctx context.Context, id uuid.UUID, amount int64) (bank.Account, error) {
	var row accountRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&row, "id = ?", id).Error; err != nil {
			return err
		}
		row.Balance += amount
		return tx.Model(&accountRow{}).
			Where("id = ?", id).
			Update("balance", row.Balance).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return bank.Account{}, bank.ErrAccountNotFound
	}
	if err != nil {
		return bank.Account{}, err
	}
	return row.toDomain(), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bank/ -run TestRepository_Deposit -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bank/account_repository.go internal/bank/account_repository_test.go
git commit -m "feat: add deposit with row locking"
```

---

## Task 7: Postgres Transfer (ordered locking)

**Files:**
- Modify: `internal/bank/account_repository.go`
- Test: `internal/bank/account_repository_test.go`

**Interfaces:**
- Produces: `func (s *Repository) Transfer(ctx, from, to uuid.UUID, amount int64) (bank.TransferResult, error)`. Locks both rows in ascending id order inside one transaction; unknown account → `bank.ErrAccountNotFound`; source funds < amount → `bank.ErrInsufficientFunds`. On success debits source, credits destination, returns both new balances.

- [ ] **Step 1: Write the failing test**

Append to `internal/bank/account_repository_test.go`:

```go
func TestRepository_Transfer_MovesFunds(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	from, err := testRepo.Create(ctx, 1000)
	require.NoError(t, err)
	to, err := testRepo.Create(ctx, 200)
	require.NoError(t, err)

	res, err := testRepo.Transfer(ctx, from.ID, to.ID, 300)
	require.NoError(t, err)
	assert.Equal(t, int64(700), res.FromBalance)
	assert.Equal(t, int64(500), res.ToBalance)

	gotFrom, _ := testRepo.Get(ctx, from.ID)
	gotTo, _ := testRepo.Get(ctx, to.ID)
	assert.Equal(t, int64(700), gotFrom.Balance)
	assert.Equal(t, int64(500), gotTo.Balance)
}

func TestRepository_Transfer_InsufficientFunds(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	from, _ := testRepo.Create(ctx, 100)
	to, _ := testRepo.Create(ctx, 0)

	_, err := testRepo.Transfer(ctx, from.ID, to.ID, 500)

	assert.ErrorIs(t, err, bank.ErrInsufficientFunds)
	gotFrom, _ := testRepo.Get(ctx, from.ID)
	assert.Equal(t, int64(100), gotFrom.Balance, "balance unchanged on failed transfer")
}

func TestRepository_Transfer_UnknownAccount(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	from, _ := testRepo.Create(ctx, 100)

	_, err := testRepo.Transfer(ctx, from.ID, uuid.New(), 10)

	assert.ErrorIs(t, err, bank.ErrAccountNotFound)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bank/ -run TestRepository_Transfer -v`
Expected: FAIL — `undefined: (*Repository).Transfer`.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/bank/account_repository.go`:

```go
func (s *Repository) Transfer(ctx context.Context, from, to uuid.UUID, amount int64) (bank.TransferResult, error) {
	var result bank.TransferResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []accountRow
		// Lock both rows in a deterministic order (ascending id) so two
		// opposing transfers can never hold locks in a cycle -> no deadlock.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", []uuid.UUID{from, to}).
			Order("id").
			Find(&rows).Error; err != nil {
			return err
		}

		byID := make(map[uuid.UUID]accountRow, len(rows))
		for _, r := range rows {
			byID[r.ID] = r
		}
		src, ok := byID[from]
		if !ok {
			return bank.ErrAccountNotFound
		}
		dst, ok := byID[to]
		if !ok {
			return bank.ErrAccountNotFound
		}
		if src.Balance < amount {
			return bank.ErrInsufficientFunds
		}

		src.Balance -= amount
		dst.Balance += amount
		if err := tx.Model(&accountRow{}).Where("id = ?", from).Update("balance", src.Balance).Error; err != nil {
			return err
		}
		if err := tx.Model(&accountRow{}).Where("id = ?", to).Update("balance", dst.Balance).Error; err != nil {
			return err
		}

		result = bank.TransferResult{
			FromAccountID: from,
			ToAccountID:   to,
			Amount:        amount,
			FromBalance:   src.Balance,
			ToBalance:     dst.Balance,
		}
		return nil
	})
	if err != nil {
		return bank.TransferResult{}, err
	}
	return result, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bank/ -run TestRepository_Transfer -v`
Expected: PASS.

- [ ] **Step 5: Refactor check**

Confirm the store fully satisfies `bank.AccountRepository`. Add a compile-time assertion at the top of `store.go`:

```go
var _ bank.AccountRepository = (*Repository)(nil)
```

Run: `go build ./internal/bank/`
Expected: compiles.

- [ ] **Step 6: Commit**

```bash
git add internal/bank/account_repository.go internal/bank/account_repository_test.go
git commit -m "feat: add atomic transfer with ordered row locking"
```

---

## Task 8: Concurrency & consistency test (the key correctness proof)

**Files:**
- Test: `internal/bank/concurrency_test.go`

**Interfaces:**
- Consumes: `testRepo`, `truncate` from `store_test.go` (same package).
- Produces: no new production code — this task proves FR6 (money conservation, no negative balances, no deadlocks) against the real database.

- [ ] **Step 1: Write the failing test**

`internal/bank/concurrency_test.go`:

```go
package bank

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pavlomaksymov/bank-account-api/internal/bank"
)

func TestTransfer_ConcurrentConservesMoney(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const n = 10
	const initial = int64(1000)
	ids := make([]uuid.UUID, n)
	for i := range ids {
		acc, err := testRepo.Create(ctx, initial)
		require.NoError(t, err)
		ids[i] = acc.ID
	}
	total := int64(n) * initial

	const workers = 50
	const perWorker = 20
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(seed)))
			for i := 0; i < perWorker; i++ {
				from := ids[r.Intn(n)]
				to := ids[r.Intn(n)]
				if from == to {
					continue
				}
				amount := int64(r.Intn(50) + 1)
				_, err := testRepo.Transfer(ctx, from, to, amount)
				if err != nil && !errors.Is(err, bank.ErrInsufficientFunds) {
					assert.NoError(t, err, "only insufficient-funds is an acceptable failure")
				}
			}
		}(w)
	}
	wg.Wait()

	var sum int64
	for _, id := range ids {
		acc, err := testRepo.Get(ctx, id)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, acc.Balance, int64(0), "no balance may go negative")
		sum += acc.Balance
	}
	assert.Equal(t, total, sum, "total money must be conserved")
}

func TestTransfer_OpposingTransfersNoDeadlock(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	a, _ := testRepo.Create(ctx, 100000)
	b, _ := testRepo.Create(ctx, 100000)

	const rounds = 200
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			_, _ = testRepo.Transfer(ctx, a.ID, b.ID, 1)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			_, _ = testRepo.Transfer(ctx, b.ID, a.ID, 1)
		}
	}()
	wg.Wait()

	gotA, _ := testRepo.Get(ctx, a.ID)
	gotB, _ := testRepo.Get(ctx, b.ID)
	assert.Equal(t, int64(200000), gotA.Balance+gotB.Balance)
}
```

- [ ] **Step 2: Run test to verify it passes (green immediately — implementation exists from Task 7)**

Run: `go test ./internal/bank/ -run 'TestTransfer_Concurrent|TestTransfer_Opposing' -race -v`
Expected: PASS with `-race` clean. If it fails on conservation or deadlocks, the locking in Task 7 is wrong — fix there before proceeding.

> Note: this is the one task where the test is green on first run because it exercises Task 7's code. Its purpose is to *prove* the concurrency contract; treat a failure here as a Task 7 regression.

- [ ] **Step 3: Commit**

```bash
git add internal/bank/concurrency_test.go
git commit -m "test: prove money conservation and deadlock-freedom under concurrency"
```

---

## Task 9: API error mapping and JSON helpers

**Files:**
- Create: `internal/api/errors.go`
- Test: `internal/api/errors_test.go`

**Interfaces:**
- Produces:
  - `var errInvalidRequest = errors.New("invalid request")` (package-level, for decode/parse failures).
  - `func writeJSON(w http.ResponseWriter, status int, body any)`.
  - `func writeError(w http.ResponseWriter, err error)` mapping sentinel errors → status + stable code, body `{"error":{"code","message"}}`.

- [ ] **Step 1: Write the failing test**

`internal/api/errors_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pavlomaksymov/bank-account-api/internal/bank"
)

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) (int, string) {
	t.Helper()
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return rec.Code, body.Error.Code
}

func TestWriteError_Mapping(t *testing.T) {
	cases := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{bank.ErrAccountNotFound, http.StatusNotFound, "account_not_found"},
		{bank.ErrInsufficientFunds, http.StatusUnprocessableEntity, "insufficient_funds"},
		{bank.ErrInvalidAmount, http.StatusUnprocessableEntity, "invalid_amount"},
		{bank.ErrNegativeBalance, http.StatusUnprocessableEntity, "invalid_amount"},
		{bank.ErrSelfTransfer, http.StatusUnprocessableEntity, "self_transfer"},
		{errInvalidRequest, http.StatusBadRequest, "invalid_request"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		writeError(rec, tc.err)
		status, code := decodeError(t, rec)
		assert.Equal(t, tc.wantStatus, status)
		assert.Equal(t, tc.wantCode, code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	}
}

func TestWriteError_UnknownIsInternal(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, assertAnError{})
	status, code := decodeError(t, rec)
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, "internal_error", code)
}

type assertAnError struct{}

func (assertAnError) Error() string { return "boom" }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestWriteError -v`
Expected: FAIL — `undefined: writeError`.

- [ ] **Step 3: Write minimal implementation**

`internal/api/errors.go`:

```go
package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/pavlomaksymov/bank-account-api/internal/bank"
)

var errInvalidRequest = errors.New("invalid request")

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal_error"
	switch {
	case errors.Is(err, bank.ErrAccountNotFound):
		status, code = http.StatusNotFound, "account_not_found"
	case errors.Is(err, bank.ErrInsufficientFunds):
		status, code = http.StatusUnprocessableEntity, "insufficient_funds"
	case errors.Is(err, bank.ErrInvalidAmount):
		status, code = http.StatusUnprocessableEntity, "invalid_amount"
	case errors.Is(err, bank.ErrNegativeBalance):
		status, code = http.StatusUnprocessableEntity, "invalid_amount"
	case errors.Is(err, bank.ErrSelfTransfer):
		status, code = http.StatusUnprocessableEntity, "self_transfer"
	case errors.Is(err, errInvalidRequest):
		status, code = http.StatusBadRequest, "invalid_request"
	}
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: err.Error()}})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestWriteError -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/errors.go internal/api/errors_test.go
git commit -m "feat: add api error mapping and json helpers"
```

---

## Task 10: API handlers, DTOs, router

**Files:**
- Create: `internal/api/dto.go`, `internal/api/handlers.go`, `internal/api/router.go`, `internal/api/handlers_test.go`

**Interfaces:**
- Consumes: `bank.Account`, `bank.TransferResult`, `bank.AccountService` (satisfies the `Service` interface below), `writeJSON`, `writeError`, `errInvalidRequest`.
- Produces:
  - `type Service interface { Create(ctx, initialBalance int64) (bank.Account, error); Get(ctx, id uuid.UUID) (bank.Account, error); Deposit(ctx, id uuid.UUID, amount int64) (bank.Account, error); Transfer(ctx, from, to uuid.UUID, amount int64) (bank.TransferResult, error) }`.
  - `func NewHandler(svc Service) *Handler` with methods `createAccount`, `getAccount`, `deposit`, `transfer`, `health`.
  - `func NewRouter(h *Handler) http.Handler` (middleware wired in Task 11; until then wrap the mux directly).

- [ ] **Step 1: Write the DTOs**

`internal/api/dto.go`:

```go
package api

import (
	"time"

	"github.com/google/uuid"
)

type createAccountRequest struct {
	InitialBalance int64 `json:"initial_balance"`
}

type accountResponse struct {
	ID        uuid.UUID `json:"id"`
	Balance   int64     `json:"balance"`
	CreatedAt time.Time `json:"created_at"`
}

type depositRequest struct {
	Amount int64 `json:"amount"`
}

type balanceResponse struct {
	ID      uuid.UUID `json:"id"`
	Balance int64     `json:"balance"`
}

type transferRequest struct {
	FromAccountID uuid.UUID `json:"from_account_id"`
	ToAccountID   uuid.UUID `json:"to_account_id"`
	Amount        int64     `json:"amount"`
}

type transferResponse struct {
	FromAccountID uuid.UUID `json:"from_account_id"`
	ToAccountID   uuid.UUID `json:"to_account_id"`
	Amount        int64     `json:"amount"`
	FromBalance   int64     `json:"from_balance"`
	ToBalance     int64     `json:"to_balance"`
}
```

- [ ] **Step 2: Write the failing test**

`internal/api/handlers_test.go`:

```go
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pavlomaksymov/bank-account-api/internal/bank"
)

type fakeService struct {
	account  bank.Account
	transfer bank.TransferResult
	err      error
}

func (f fakeService) Create(context.Context, int64) (bank.Account, error) {
	return f.account, f.err
}
func (f fakeService) Get(context.Context, uuid.UUID) (bank.Account, error) {
	return f.account, f.err
}
func (f fakeService) Deposit(context.Context, uuid.UUID, int64) (bank.Account, error) {
	return f.account, f.err
}
func (f fakeService) Transfer(context.Context, uuid.UUID, uuid.UUID, int64) (bank.TransferResult, error) {
	return f.transfer, f.err
}

func TestCreate_Created(t *testing.T) {
	id := uuid.New()
	h := NewHandler(fakeService{account: bank.Account{ID: id, Balance: 1000}})
	srv := NewRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(`{"initial_balance":1000}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Contains(t, rec.Body.String(), id.String())
}

func TestCreate_MalformedJSON(t *testing.T) {
	h := NewHandler(fakeService{})
	srv := NewRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(`{bad`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGet_BadUUID(t *testing.T) {
	h := NewHandler(fakeService{})
	srv := NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/accounts/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTransfer_InsufficientFunds_MapsTo422(t *testing.T) {
	h := NewHandler(fakeService{err: bank.ErrInsufficientFunds})
	srv := NewRouter(h)

	body := `{"from_account_id":"` + uuid.New().String() + `","to_account_id":"` + uuid.New().String() + `","amount":50}`
	req := httptest.NewRequest(http.MethodPost, "/transfers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestHealth_OK(t *testing.T) {
	h := NewHandler(fakeService{})
	srv := NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/api/ -run 'TestCreate|TestGet|TestTransfer_Insufficient|TestHealth' -v`
Expected: FAIL — `undefined: NewHandler` / `NewRouter`.

- [ ] **Step 4: Write minimal implementation**

`internal/api/handlers.go`:

```go
package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/pavlomaksymov/bank-account-api/internal/bank"
)

type Service interface {
	Create(ctx context.Context, initialBalance int64) (bank.Account, error)
	Get(ctx context.Context, id uuid.UUID) (bank.Account, error)
	Deposit(ctx context.Context, id uuid.UUID, amount int64) (bank.Account, error)
	Transfer(ctx context.Context, from, to uuid.UUID, amount int64) (bank.TransferResult, error)
}

type Handler struct {
	svc Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errInvalidRequest
	}
	return nil
}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := decode(r, &req); err != nil {
		writeError(w, err)
		return
	}
	acc, err := h.svc.Create(r.Context(), req.InitialBalance)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, accountResponse{ID: acc.ID, Balance: acc.Balance, CreatedAt: acc.CreatedAt})
}

func (h *Handler) getAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, errInvalidRequest)
		return
	}
	acc, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, accountResponse{ID: acc.ID, Balance: acc.Balance, CreatedAt: acc.CreatedAt})
}

func (h *Handler) deposit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, errInvalidRequest)
		return
	}
	var req depositRequest
	if err := decode(r, &req); err != nil {
		writeError(w, err)
		return
	}
	acc, err := h.svc.Deposit(r.Context(), id, req.Amount)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, balanceResponse{ID: acc.ID, Balance: acc.Balance})
}

func (h *Handler) transfer(w http.ResponseWriter, r *http.Request) {
	var req transferRequest
	if err := decode(r, &req); err != nil {
		writeError(w, err)
		return
	}
	res, err := h.svc.Transfer(r.Context(), req.FromAccountID, req.ToAccountID, req.Amount)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, transferResponse{
		FromAccountID: res.FromAccountID,
		ToAccountID:   res.ToAccountID,
		Amount:        res.Amount,
		FromBalance:   res.FromBalance,
		ToBalance:     res.ToBalance,
	})
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

`internal/api/router.go`:

```go
package api

import "net/http"

func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /accounts", h.createAccount)
	mux.HandleFunc("GET /accounts/{id}", h.getAccount)
	mux.HandleFunc("POST /accounts/{id}/deposits", h.deposit)
	mux.HandleFunc("POST /transfers", h.transfer)
	mux.HandleFunc("GET /healthz", h.health)
	return mux
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/api/ -v`
Expected: PASS.

- [ ] **Step 6: Refactor check**

Add compile-time proof that `*bank.AccountService` satisfies `api.Service` — put it in `handlers.go`:

```go
// keeps the api.Service contract in lockstep with bank.AccountService
var _ Service = (*bank.AccountService)(nil)
```

Run: `go build ./...`
Expected: compiles.

- [ ] **Step 7: Commit**

```bash
git add internal/api/dto.go internal/api/handlers.go internal/api/router.go internal/api/handlers_test.go
git commit -m "feat: add http handlers, dtos and router"
```

---

## Task 11: Middleware (request-id, logger, recoverer)

**Files:**
- Create: `internal/api/middleware.go`
- Modify: `internal/api/router.go`
- Test: `internal/api/middleware_test.go`

**Interfaces:**
- Produces: `func withMiddleware(next http.Handler) http.Handler` composing requestID → logger → recoverer. Recoverer converts a handler panic into a `500` JSON error. RequestID sets an `X-Request-Id` response header.

- [ ] **Step 1: Write the failing test**

`internal/api/middleware_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecoverer_TurnsPanicInto500(t *testing.T) {
	panicky := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	})
	h := withMiddleware(panicky)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "internal_error")
}

func TestRequestID_SetsHeader(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := withMiddleware(ok)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.NotEmpty(t, rec.Header().Get("X-Request-Id"))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run 'TestRecoverer|TestRequestID' -v`
Expected: FAIL — `undefined: withMiddleware`.

- [ ] **Step 3: Write minimal implementation**

`internal/api/middleware.go`:

```go
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func withMiddleware(next http.Handler) http.Handler {
	return requestID(logger(recoverer(next)))
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", w.Header().Get("X-Request-Id"),
		)
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rv := recover(); rv != nil {
				slog.Error("panic recovered", "value", rv, "path", r.URL.Path)
				writeError(w, errInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
```

Add the internal error sentinel to `internal/api/errors.go` (so recoverer maps to `internal_error`/500 without leaking details):

```go
var errInternal = errors.New("internal error")
```

`writeError` already defaults unmatched errors to `500`/`internal_error`, so `errInternal` needs no new case.

- [ ] **Step 4: Wire middleware into the router**

In `internal/api/router.go`, change the final return:

```go
	return withMiddleware(mux)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/ -v`
Expected: PASS (handler tests still pass through the middleware chain).

- [ ] **Step 6: Commit**

```bash
git add internal/api/middleware.go internal/api/router.go internal/api/errors.go internal/api/middleware_test.go
git commit -m "feat: add request-id, logging and panic-recovery middleware"
```

---

## Task 12: Full-stack integration test + main wiring with graceful shutdown

**Files:**
- Create: `internal/api/integration_test.go`, `cmd/api/main.go`

**Interfaces:**
- Consumes: `config.Load`, `bank.NewRepository`, `migrations.Up`, `bank.NewAccountService`, `api.NewHandler`, `api.NewRouter`.
- Produces: a runnable binary at `cmd/api` and a test proving the whole stack (HTTP → service → real Postgres) works end to end.

- [ ] **Step 1: Write the failing integration test**

`internal/api/integration_test.go`:

```go
package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/pavlomaksymov/bank-account-api/internal/api"
	"github.com/pavlomaksymov/bank-account-api/internal/bank"
	"github.com/pavlomaksymov/bank-account-api/migrations"
)

func newStack(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("bank"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	gdb, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	require.NoError(t, migrations.Up(sqlDB))

	svc := bank.NewAccountService(bank.NewRepository(gdb))
	return api.NewRouter(api.NewHandler(svc))
}

func TestEndToEnd_CreateDepositTransfer(t *testing.T) {
	srv := newStack(t)

	post := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return rec
	}

	a := post("/accounts", `{"initial_balance":1000}`)
	require.Equal(t, http.StatusCreated, a.Code)
	b := post("/accounts", `{"initial_balance":0}`)
	require.Equal(t, http.StatusCreated, b.Code)

	idA := extractID(t, a.Body.Bytes())
	idB := extractID(t, b.Body.Bytes())

	dep := post("/accounts/"+idA+"/deposits", `{"amount":500}`)
	require.Equal(t, http.StatusOK, dep.Code)

	tr := post("/transfers", `{"from_account_id":"`+idA+`","to_account_id":"`+idB+`","amount":600}`)
	require.Equal(t, http.StatusOK, tr.Code)

	getB := httptest.NewRecorder()
	srv.ServeHTTP(getB, httptest.NewRequest(http.MethodGet, "/accounts/"+idB, nil))
	require.Contains(t, getB.Body.String(), `"balance":600`)
}
```

Add the small helper (same file):

```go
func extractID(t *testing.T, body []byte) string {
	t.Helper()
	var parsed struct {
		ID string `json:"id"`
	}
	require.NoError(t, jsonUnmarshal(body, &parsed))
	return parsed.ID
}
```

And at the top of the file add a tiny wrapper to avoid importing encoding/json twice conceptually — actually use encoding/json directly:

```go
import "encoding/json"

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestEndToEnd -v`
Expected: FAIL to compile until `cmd/api/main.go` exists is NOT required (test only imports packages, not main). It should actually PASS once the packages compile. If it fails, it reveals a real wiring bug — fix it. (This is the acceptance test for the whole stack.)

- [ ] **Step 3: Write `cmd/api/main.go`**

```go
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/pavlomaksymov/bank-account-api/internal/api"
	"github.com/pavlomaksymov/bank-account-api/internal/bank"
	"github.com/pavlomaksymov/bank-account-api/internal/config"
	"github.com/pavlomaksymov/bank-account-api/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()

	gdb, err := gorm.Open(gormpg.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		return err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	if err := migrations.Up(sqlDB); err != nil {
		return err
	}

	svc := bank.NewAccountService(bank.NewRepository(gdb))
	router := api.NewRouter(api.NewHandler(svc))
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: router}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	_ = sqlDB.Close()
	return nil
}
```

- [ ] **Step 4: Run everything**

Run: `go build ./... && go test ./internal/api/ -run TestEndToEnd -v`
Expected: build succeeds; end-to-end test PASSES.

- [ ] **Step 5: Commit**

```bash
git add internal/api/integration_test.go cmd/api/main.go
git commit -m "feat: wire main with graceful shutdown and add end-to-end test"
```

---

## Task 13: Docker, compose, Makefile, smoke script

**Files:**
- Create: `Dockerfile`, `compose.yaml`, `scripts/smoke.sh`
- Modify: `Makefile`

**Interfaces:**
- Produces: `docker compose up` boots Postgres + API (migrations run on startup); `scripts/smoke.sh` drives the running API with curl and asserts outcomes.

- [ ] **Step 1: Write the `Dockerfile`**

```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
```

- [ ] **Step 2: Write `compose.yaml`**

```yaml
services:
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: bank
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres -d bank"]
      interval: 2s
      timeout: 3s
      retries: 15
    ports:
      - "5432:5432"

  api:
    build: .
    environment:
      DATABASE_URL: postgres://postgres:postgres@db:5432/bank?sslmode=disable
      PORT: "8080"
    depends_on:
      db:
        condition: service_healthy
    ports:
      - "8080:8080"
```

- [ ] **Step 3: Flesh out the `Makefile`**

```makefile
.PHONY: build run test test-short up down logs smoke tidy migrate-create

build:
	CGO_ENABLED=0 go build -o bin/api ./cmd/api

run:
	go run ./cmd/api

test:
	go test ./... -race

test-short:
	go test ./... -short

up:
	docker compose up --build -d

down:
	docker compose down -v

logs:
	docker compose logs -f api

smoke:
	./scripts/smoke.sh

tidy:
	go mod tidy

migrate-create:
	goose -dir migrations create $(name) sql
```

- [ ] **Step 4: Write `scripts/smoke.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE_URL:-http://localhost:8080}"

echo "health:"
curl -fsS "$BASE/healthz"; echo

a=$(curl -fsS -X POST "$BASE/accounts" -d '{"initial_balance":1000}' | sed -E 's/.*"id":"([^"]+)".*/\1/')
b=$(curl -fsS -X POST "$BASE/accounts" -d '{"initial_balance":0}'    | sed -E 's/.*"id":"([^"]+)".*/\1/')
echo "created A=$a B=$b"

echo "deposit 500 -> A:"
curl -fsS -X POST "$BASE/accounts/$a/deposits" -d '{"amount":500}'; echo

echo "transfer 600 A -> B:"
curl -fsS -X POST "$BASE/transfers" -d "{\"from_account_id\":\"$a\",\"to_account_id\":\"$b\",\"amount\":600}"; echo

echo "B balance (expect 600):"
curl -fsS "$BASE/accounts/$b"; echo

echo "overdraft attempt (expect 422):"
curl -s -o /dev/null -w "%{http_code}\n" -X POST "$BASE/transfers" \
  -d "{\"from_account_id\":\"$b\",\"to_account_id\":\"$a\",\"amount\":999999}"
```

- [ ] **Step 5: Make the script executable and verify the stack end-to-end**

```bash
chmod +x scripts/smoke.sh
docker compose up --build -d
# wait for readiness, then:
./scripts/smoke.sh
docker compose down -v
```

Expected: smoke script prints created ids, `B` balance `600`, and overdraft attempt prints `422`.

- [ ] **Step 6: Commit**

```bash
git add Dockerfile compose.yaml Makefile scripts/smoke.sh
git commit -m "chore: add docker, compose, makefile and smoke script"
```

---

## Task 14: README and trade-offs (review checkpoint)

**Files:**
- Modify: `README.md`

**Interfaces:**
- Produces: setup/build/test/run instructions, API reference with curl examples, and a decisions/trade-offs/limitations section.

> **Review gate (per author instruction):** the trade-offs / limitations prose must be reviewed with the author before this task's commit. Draft it, then pause for approval.

- [ ] **Step 1: Write `README.md`** covering, at minimum:
  - One-paragraph overview + the three operations.
  - **Requirements we defined** (link to `docs/superpowers/specs/2026-08-19-bank-account-api-design.md`).
  - **Run:** `docker compose up --build` (migrations auto-run). **Build:** `make build`. **Test:** `make test` (needs Docker for integration/concurrency tests; `make test-short` skips them). **Config:** `DATABASE_URL`, `PORT`.
  - **API reference:** each endpoint with a curl example and the success/error status codes, plus the error body shape.
  - **Design decisions & trade-offs:** copy the spec's decisions (balance-only + deferred ledger, deferred idempotency, GORM vs pgx incl. the readability/maintainability upside, `422` vs `409`, pessimistic ordered locks, single currency, testcontainers/Docker, goose-owns-schema).
  - **Security / multi-tenancy:** the RLS note (documented, not implemented).
  - **Future improvements** and **interview discussion points**.

- [ ] **Step 2: Author review of trade-offs section**

Pause and get the author's sign-off on the trade-offs / limitations wording. Apply edits.

- [ ] **Step 3: Verify docs match reality**

Run: `go test ./... -short` and confirm every curl example in the README matches an actual route/status. Fix drift.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add readme with setup, api reference and trade-offs"
```

---

## Self-Review

**1. Spec coverage:**
- FR1 create + non-negative → Task 4 (validation) + Task 5 (persist). ✓
- FR2 get balance → Task 5. ✓
- FR3 deposit → Task 4 + Task 6. ✓
- FR4 atomic transfer → Task 7. ✓
- FR5 insufficient funds, no partial state → Task 7 (tx + rollback). ✓
- FR6 conservation under concurrency → Task 8. ✓
- FR7 int64 minor units, reject non-positive → Tasks 4, 6, 7. ✓
- FR8 consistent JSON errors → Task 9. ✓
- NFR1/2 consistency + concurrency → Tasks 7, 8. ✓
- NFR3 runnable → Task 13. ✓
- NFR4 buildable → Tasks 1, 13. ✓
- NFR5 testable → every task; `make test` Task 13. ✓
- NFR6 portable → Tasks 1, 13 (env config, compose). ✓
- NFR7 observability → Task 11 (logging, request-id) + `/healthz` Task 10. ✓
- NFR8 security (validation, params) → Tasks 4/10; RLS documented Task 14. ✓
- NFR9 maintainability → package split throughout. ✓
- NFR10 graceful lifecycle → Task 12. ✓
- API surface (5 endpoints, status codes, error body) → Tasks 9–11. ✓

**2. Placeholder scan:** no TBD/TODO; every code and test step contains complete code. ✓

**3. Type consistency:** `AccountRepository` / `api.Service` method sets are identical and match `*bank.AccountService` and `*bank.Repository` (compile-time assertions in Tasks 7 and 10). `Account`/`TransferResult` field names consistent across `bank`, `api`. ✓
