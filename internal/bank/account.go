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
