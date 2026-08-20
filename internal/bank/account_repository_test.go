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

	"github.com/pavlomaksymov/bank-account-api/migrations"
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
	if err := migrations.Up(sqlDB); err != nil {
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

	assert.ErrorIs(t, err, ErrAccountNotFound)
}

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

	assert.ErrorIs(t, err, ErrAccountNotFound)
}

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

	assert.ErrorIs(t, err, ErrInsufficientFunds)
	gotFrom, _ := testRepo.Get(ctx, from.ID)
	assert.Equal(t, int64(100), gotFrom.Balance, "balance unchanged on failed transfer")
}

func TestRepository_Transfer_UnknownAccount(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	from, _ := testRepo.Create(ctx, 100)

	_, err := testRepo.Transfer(ctx, from.ID, uuid.New(), 10)

	assert.ErrorIs(t, err, ErrAccountNotFound)
}

func TestRepository_Transfer_SelfTransfer(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	acc, err := testRepo.Create(ctx, 100)
	require.NoError(t, err)

	_, err = testRepo.Transfer(ctx, acc.ID, acc.ID, 50)
	assert.ErrorIs(t, err, ErrSelfTransfer)

	got, err := testRepo.Get(ctx, acc.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(100), got.Balance, "self-transfer must not create money")
}

func TestRepository_Transfer_NonPositiveAmount(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	from, err := testRepo.Create(ctx, 1000)
	require.NoError(t, err)
	to, err := testRepo.Create(ctx, 0)
	require.NoError(t, err)

	_, err = testRepo.Transfer(ctx, from.ID, to.ID, 0)
	assert.ErrorIs(t, err, ErrInvalidAmount)

	_, err = testRepo.Transfer(ctx, from.ID, to.ID, -5)
	assert.ErrorIs(t, err, ErrInvalidAmount)

	got, err := testRepo.Get(ctx, from.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1000), got.Balance, "balance unchanged after rejected transfers")
}

func TestRepository_Deposit_NonPositiveAmount(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	acc, err := testRepo.Create(ctx, 100)
	require.NoError(t, err)

	_, err = testRepo.Deposit(ctx, acc.ID, 0)
	assert.ErrorIs(t, err, ErrInvalidAmount)

	_, err = testRepo.Deposit(ctx, acc.ID, -5)
	assert.ErrorIs(t, err, ErrInvalidAmount)

	got, err := testRepo.Get(ctx, acc.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(100), got.Balance, "balance unchanged after rejected deposits")
}
