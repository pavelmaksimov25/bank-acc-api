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
