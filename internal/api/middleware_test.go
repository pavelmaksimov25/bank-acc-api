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
