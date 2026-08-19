package bank

import (
	"context"

	"github.com/google/uuid"
)

type AccountService struct {
	repo AccountRepository
}

func NewAccountService(repo AccountRepository) *AccountService {
	return &AccountService{repo: repo}
}

func (s *AccountService) Create(ctx context.Context, initialBalance int64) (Account, error) {
	if initialBalance < 0 {
		return Account{}, ErrNegativeBalance
	}
	return s.repo.Create(ctx, initialBalance)
}

func (s *AccountService) Get(ctx context.Context, id uuid.UUID) (Account, error) {
	return s.repo.Get(ctx, id)
}

func (s *AccountService) Deposit(ctx context.Context, id uuid.UUID, amount int64) (Account, error) {
	if amount <= 0 {
		return Account{}, ErrInvalidAmount
	}
	return s.repo.Deposit(ctx, id, amount)
}

func (s *AccountService) Transfer(ctx context.Context, from, to uuid.UUID, amount int64) (TransferResult, error) {
	if amount <= 0 {
		return TransferResult{}, ErrInvalidAmount
	}
	if from == to {
		return TransferResult{}, ErrSelfTransfer
	}
	return s.repo.Transfer(ctx, from, to, amount)
}
