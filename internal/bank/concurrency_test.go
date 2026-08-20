package bank

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/google/uuid"
)

func TestTransfer_ConcurrentConservesMoney(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const n = 10
	const initial = int64(1000)
	ids := make([]uuid.UUID, n)
	for i := range ids {
		acc, err := testRepo.Create(ctx, initial)
		require.NoError(t, err)
		ids[i] = acc.ID
	}
	total := int64(n) * initial

	const workers = 50
	const perWorker = 20
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(seed)))
			for i := 0; i < perWorker; i++ {
				from := ids[r.Intn(n)]
				to := ids[r.Intn(n)]
				if from == to {
					continue
				}
				amount := int64(r.Intn(50) + 1)
				_, err := testRepo.Transfer(ctx, from, to, amount)
				if err != nil && !errors.Is(err, ErrInsufficientFunds) {
					assert.NoError(t, err, "only insufficient-funds is an acceptable failure")
				}
			}
		}(w)
	}
	wg.Wait()

	var sum int64
	for _, id := range ids {
		acc, err := testRepo.Get(ctx, id)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, acc.Balance, int64(0), "no balance may go negative")
		sum += acc.Balance
	}
	assert.Equal(t, total, sum, "total money must be conserved")
}

func TestTransfer_OpposingTransfersNoDeadlock(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	a, _ := testRepo.Create(ctx, 100000)
	b, _ := testRepo.Create(ctx, 100000)

	const rounds = 200
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			_, _ = testRepo.Transfer(ctx, a.ID, b.ID, 1)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			_, _ = testRepo.Transfer(ctx, b.ID, a.ID, 1)
		}
	}()
	wg.Wait()

	gotA, _ := testRepo.Get(ctx, a.ID)
	gotB, _ := testRepo.Get(ctx, b.ID)
	assert.Equal(t, int64(200000), gotA.Balance+gotB.Balance)
}
