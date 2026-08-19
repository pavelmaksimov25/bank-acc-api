package bank

import (
	"context"

	"github.com/google/uuid"
)

type AccountRepository interface {
	Create(ctx context.Context, initialBalance int64) (Account, error)
	Get(ctx context.Context, id uuid.UUID) (Account, error)
	Deposit(ctx context.Context, id uuid.UUID, amount int64) (Account, error)
	Transfer(ctx context.Context, from, to uuid.UUID, amount int64) (TransferResult, error)
}
