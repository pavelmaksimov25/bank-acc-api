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
