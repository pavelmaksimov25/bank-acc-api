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

func (s *Repository) Deposit(ctx context.Context, id uuid.UUID, amount int64) (Account, error) {
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
