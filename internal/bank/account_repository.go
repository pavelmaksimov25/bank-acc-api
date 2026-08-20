package bank

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AccountRepository interface {
	Create(ctx context.Context, initialBalance int64) (Account, error)
	Get(ctx context.Context, id uuid.UUID) (Account, error)
	Deposit(ctx context.Context, id uuid.UUID, amount int64) (Account, error)
	Transfer(ctx context.Context, from, to uuid.UUID, amount int64) (TransferResult, error)
}

type accountRow struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	Balance   int64     `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null"`
}

func (accountRow) TableName() string { return "accounts" }

func (r accountRow) toDomain() Account {
	return Account{ID: r.ID, Balance: r.Balance, CreatedAt: r.CreatedAt}
}

type Repository struct {
	db *gorm.DB
}

var _ AccountRepository = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (s *Repository) Create(ctx context.Context, initialBalance int64) (Account, error) {
	row := accountRow{ID: uuid.New(), Balance: initialBalance}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return Account{}, err
	}
	return row.toDomain(), nil
}

func (s *Repository) Get(ctx context.Context, id uuid.UUID) (Account, error) {
	var row accountRow
	err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Account{}, ErrAccountNotFound
	}
	if err != nil {
		return Account{}, err
	}
	return row.toDomain(), nil
}

func (s *Repository) Transfer(ctx context.Context, from, to uuid.UUID, amount int64) (TransferResult, error) {
	if amount <= 0 {
		return TransferResult{}, ErrInvalidAmount
	}
	if from == to {
		return TransferResult{}, ErrSelfTransfer
	}
	var result TransferResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []accountRow
		// Lock both rows in ascending id order so two opposing transfers
		// can never hold locks in a cycle — no deadlock.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", []uuid.UUID{from, to}).
			Order("id").
			Find(&rows).Error; err != nil {
			return err
		}

		src, dst, err := s.resolveTransferAccounts(rows, from, to, amount)
		if err != nil {
			return err
		}

		src.Balance -= amount
		dst.Balance += amount
		if err := tx.Model(&accountRow{}).Where("id = ?", from).Update("balance", src.Balance).Error; err != nil {
			return err
		}
		if err := tx.Model(&accountRow{}).Where("id = ?", to).Update("balance", dst.Balance).Error; err != nil {
			return err
		}

		result = TransferResult{
			FromAccountID: from,
			ToAccountID:   to,
			Amount:        amount,
			FromBalance:   src.Balance,
			ToBalance:     dst.Balance,
		}
		return nil
	})
	if err != nil {
		return TransferResult{}, err
	}
	return result, nil
}

func (s *Repository) resolveTransferAccounts(rows []accountRow, from, to uuid.UUID, amount int64) (accountRow, accountRow, error) {
	byID := make(map[uuid.UUID]accountRow, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	src, ok := byID[from]
	if !ok {
		return accountRow{}, accountRow{}, ErrAccountNotFound
	}
	dst, ok := byID[to]
	if !ok {
		return accountRow{}, accountRow{}, ErrAccountNotFound
	}
	if src.Balance < amount {
		return accountRow{}, accountRow{}, ErrInsufficientFunds
	}
	return src, dst, nil
}

func (s *Repository) Deposit(ctx context.Context, id uuid.UUID, amount int64) (Account, error) {
	if amount <= 0 {
		return Account{}, ErrInvalidAmount
	}
	var row accountRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&row, "id = ?", id).Error; err != nil {
			return err
		}
		row.Balance += amount
		return tx.Model(&accountRow{}).
			Where("id = ?", id).
			Update("balance", row.Balance).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Account{}, ErrAccountNotFound
	}
	if err != nil {
		return Account{}, err
	}
	return row.toDomain(), nil
}
