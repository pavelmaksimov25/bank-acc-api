package bank

import "errors"

var (
	ErrAccountNotFound   = errors.New("account not found")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrInvalidAmount     = errors.New("amount must be positive")
	ErrNegativeBalance   = errors.New("initial balance must not be negative")
	ErrSelfTransfer      = errors.New("cannot transfer to the same account")
)
