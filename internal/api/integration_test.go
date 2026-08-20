package api_test

import (
	"context"
	"encoding/json"
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

func extractID(t *testing.T, body []byte) string {
	t.Helper()
	var parsed struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(body, &parsed))
	return parsed.ID
}
