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
