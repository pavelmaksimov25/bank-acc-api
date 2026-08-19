package bank

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	createCalls   int
	depositCalls  int
	transferCalls int
}

func (f *fakeRepository) Create(_ context.Context, initialBalance int64) (Account, error) {
	f.createCalls++
	return Account{ID: uuid.New(), Balance: initialBalance}, nil
}
func (f *fakeRepository) Get(_ context.Context, id uuid.UUID) (Account, error) {
	return Account{ID: id}, nil
}
func (f *fakeRepository) Deposit(_ context.Context, id uuid.UUID, amount int64) (Account, error) {
	f.depositCalls++
	return Account{ID: id, Balance: amount}, nil
}
func (f *fakeRepository) Transfer(_ context.Context, from, to uuid.UUID, amount int64) (TransferResult, error) {
	f.transferCalls++
	return TransferResult{FromAccountID: from, ToAccountID: to, Amount: amount}, nil
}

func TestAccountService_Create_RejectsNegativeBalance(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewAccountService(repo)

	_, err := svc.Create(context.Background(), -1)

	assert.ErrorIs(t, err, ErrNegativeBalance)
	assert.Equal(t, 0, repo.createCalls, "repository must not be called on invalid input")
}

func TestAccountService_Create_AllowsZeroAndPositive(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewAccountService(repo)

	_, err := svc.Create(context.Background(), 0)
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, 2, repo.createCalls)
}

func TestAccountService_Deposit_RejectsNonPositive(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewAccountService(repo)

	_, err := svc.Deposit(context.Background(), uuid.New(), 0)

	assert.ErrorIs(t, err, ErrInvalidAmount)
	assert.Equal(t, 0, repo.depositCalls)
}

func TestAccountService_Transfer_RejectsNonPositive(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewAccountService(repo)

	_, err := svc.Transfer(context.Background(), uuid.New(), uuid.New(), -5)

	assert.ErrorIs(t, err, ErrInvalidAmount)
	assert.Equal(t, 0, repo.transferCalls)
}

func TestAccountService_Transfer_RejectsSelfTransfer(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewAccountService(repo)
	id := uuid.New()

	_, err := svc.Transfer(context.Background(), id, id, 10)

	assert.ErrorIs(t, err, ErrSelfTransfer)
	assert.Equal(t, 0, repo.transferCalls)
}

func TestAccountService_Transfer_DelegatesWhenValid(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewAccountService(repo)

	_, err := svc.Transfer(context.Background(), uuid.New(), uuid.New(), 10)

	require.NoError(t, err)
	assert.Equal(t, 1, repo.transferCalls)
}
