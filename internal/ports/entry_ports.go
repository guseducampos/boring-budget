package ports

import (
	"context"
	"database/sql"

	"boring-budget/internal/domain"
)

type EntryRepository interface {
	Add(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error)
	Update(ctx context.Context, input domain.EntryUpdateInput) (domain.Entry, error)
	List(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error)
	Delete(ctx context.Context, id int64) (domain.EntryDeleteResult, error)
}

type EntryCapLookup interface {
	GetByMonth(ctx context.Context, monthKey string) (domain.MonthlyCap, error)
	GetExpenseTotalByMonthAndCurrency(ctx context.Context, monthKey, currencyCode string) (int64, error)
}

type EntryRepositoryTxBinder interface {
	BindTx(tx *sql.Tx) EntryRepository
}

type EntryCapLookupTxBinder interface {
	BindTx(tx *sql.Tx) EntryCapLookup
}
