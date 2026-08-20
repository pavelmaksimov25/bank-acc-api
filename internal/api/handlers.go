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

// keeps the api.Service contract in lockstep with bank.AccountService
var _ Service = (*bank.AccountService)(nil)

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
