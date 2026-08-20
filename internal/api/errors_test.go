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
