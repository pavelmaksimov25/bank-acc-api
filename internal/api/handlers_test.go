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
