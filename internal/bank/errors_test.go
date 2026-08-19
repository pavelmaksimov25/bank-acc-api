package bank

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSentinelErrors_HaveStableMessages(t *testing.T) {
	assert.EqualError(t, ErrAccountNotFound, "account not found")
	assert.EqualError(t, ErrInsufficientFunds, "insufficient funds")
	assert.EqualError(t, ErrInvalidAmount, "amount must be positive")
	assert.EqualError(t, ErrNegativeBalance, "initial balance must not be negative")
	assert.EqualError(t, ErrSelfTransfer, "cannot transfer to the same account")
}
