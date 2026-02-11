package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"budgetto/internal/domain"
	queries "budgetto/internal/store/sqlite/sqlc"
)

type CapRepo struct {
	db      *sql.DB
	queries *queries.Queries
}

func NewCapRepo(db *sql.DB) *CapRepo {
	return &CapRepo{
		db:      db,
		queries: queries.New(db),
	}
}

func (r *CapRepo) Set(ctx context.Context, input domain.CapSetInput) (domain.MonthlyCap, domain.MonthlyCapChange, error) {
	if r.db == nil {
		return domain.MonthlyCap{}, domain.MonthlyCapChange{}, fmt.Errorf("set cap: db is nil")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.MonthlyCap{}, domain.MonthlyCapChange{}, fmt.Errorf("set cap begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	qtx := r.queries.WithTx(tx)

	existing, err := qtx.GetMonthlyCapByMonthKey(ctx, input.MonthKey)
	hasExisting := err == nil
	if err != nil && err != sql.ErrNoRows {
		return domain.MonthlyCap{}, domain.MonthlyCapChange{}, fmt.Errorf("set cap load existing: %w", err)
	}

	nowUTC := time.Now().UTC().Format(time.RFC3339Nano)
	oldAmount := sql.NullInt64{}

	if hasExisting {
		oldAmount = sql.NullInt64{Int64: existing.AmountMinor, Valid: true}

		updateResult, err := qtx.UpdateMonthlyCapByMonthKey(ctx, queries.UpdateMonthlyCapByMonthKeyParams{
			AmountMinor:  input.AmountMinor,
			CurrencyCode: input.CurrencyCode,
			UpdatedAtUtc: nowUTC,
			MonthKey:     input.MonthKey,
		})
		if err != nil {
			return domain.MonthlyCap{}, domain.MonthlyCapChange{}, fmt.Errorf("set cap update: %w", err)
		}

		rowsAffected, err := updateResult.RowsAffected()
		if err != nil {
			return domain.MonthlyCap{}, domain.MonthlyCapChange{}, fmt.Errorf("set cap update rows: %w", err)
		}
		if rowsAffected == 0 {
			return domain.MonthlyCap{}, domain.MonthlyCapChange{}, domain.ErrCapNotFound
		}
	} else {
		if _, err := qtx.CreateMonthlyCap(ctx, queries.CreateMonthlyCapParams{
			MonthKey:     input.MonthKey,
			AmountMinor:  input.AmountMinor,
			CurrencyCode: input.CurrencyCode,
			UpdatedAtUtc: nowUTC,
		}); err != nil {
			return domain.MonthlyCap{}, domain.MonthlyCapChange{}, fmt.Errorf("set cap create: %w", err)
		}
	}

	changeResult, err := qtx.CreateMonthlyCapChange(ctx, queries.CreateMonthlyCapChangeParams{
		MonthKey:       input.MonthKey,
		OldAmountMinor: oldAmount,
		NewAmountMinor: input.AmountMinor,
		CurrencyCode:   input.CurrencyCode,
		ChangedAtUtc:   nowUTC,
	})
	if err != nil {
		return domain.MonthlyCap{}, domain.MonthlyCapChange{}, fmt.Errorf("set cap create change: %w", err)
	}

	changeID, err := changeResult.LastInsertId()
	if err != nil {
		return domain.MonthlyCap{}, domain.MonthlyCapChange{}, fmt.Errorf("set cap read change id: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return domain.MonthlyCap{}, domain.MonthlyCapChange{}, fmt.Errorf("set cap commit: %w", err)
	}

	currentCap, err := r.GetByMonth(ctx, input.MonthKey)
	if err != nil {
		return domain.MonthlyCap{}, domain.MonthlyCapChange{}, err
	}

	capChange := domain.MonthlyCapChange{
		ID:             changeID,
		MonthKey:       input.MonthKey,
		NewAmountMinor: input.AmountMinor,
		CurrencyCode:   input.CurrencyCode,
		ChangedAtUTC:   nowUTC,
	}
	if oldAmount.Valid {
		old := oldAmount.Int64
		capChange.OldAmountMinor = &old
	}

	return currentCap, capChange, nil
}

func (r *CapRepo) GetByMonth(ctx context.Context, monthKey string) (domain.MonthlyCap, error) {
	if r.db == nil {
		return domain.MonthlyCap{}, fmt.Errorf("get cap: db is nil")
	}

	row, err := r.queries.GetMonthlyCapByMonthKey(ctx, monthKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.MonthlyCap{}, domain.ErrCapNotFound
		}
		return domain.MonthlyCap{}, fmt.Errorf("get cap by month: %w", err)
	}

	return mapSQLCCapToDomain(row), nil
}

func (r *CapRepo) ListChangesByMonth(ctx context.Context, monthKey string) ([]domain.MonthlyCapChange, error) {
	if r.db == nil {
		return nil, fmt.Errorf("list cap changes: db is nil")
	}

	rows, err := r.queries.ListMonthlyCapChangesByMonthKey(ctx, monthKey)
	if err != nil {
		return nil, fmt.Errorf("list cap changes: %w", err)
	}

	changes := make([]domain.MonthlyCapChange, 0, len(rows))
	for _, row := range rows {
		changes = append(changes, mapSQLCCapChangeToDomain(row))
	}
	return changes, nil
}

func (r *CapRepo) GetExpenseTotalByMonthAndCurrency(ctx context.Context, monthKey, currencyCode string) (int64, error) {
	if r.db == nil {
		return 0, fmt.Errorf("sum expenses by month and currency: db is nil")
	}

	monthStartUTC, monthEndUTC, err := domain.MonthRangeUTC(monthKey)
	if err != nil {
		return 0, err
	}

	total, err := r.queries.SumActiveExpensesByMonthAndCurrency(ctx, queries.SumActiveExpensesByMonthAndCurrencyParams{
		CurrencyCode:         currencyCode,
		TransactionDateUtc:   monthStartUTC,
		TransactionDateUtc_2: monthEndUTC,
	})
	if err != nil {
		return 0, fmt.Errorf("sum expenses by month and currency: %w", err)
	}

	return total, nil
}

func mapSQLCCapToDomain(row queries.MonthlyCap) domain.MonthlyCap {
	return domain.MonthlyCap{
		ID:           row.ID,
		MonthKey:     row.MonthKey,
		AmountMinor:  row.AmountMinor,
		CurrencyCode: row.CurrencyCode,
		CreatedAtUTC: row.CreatedAtUtc,
		UpdatedAtUTC: row.UpdatedAtUtc,
	}
}

func mapSQLCCapChangeToDomain(row queries.MonthlyCapChange) domain.MonthlyCapChange {
	change := domain.MonthlyCapChange{
		ID:             row.ID,
		MonthKey:       row.MonthKey,
		NewAmountMinor: row.NewAmountMinor,
		CurrencyCode:   row.CurrencyCode,
		ChangedAtUTC:   row.ChangedAtUtc,
	}

	if row.OldAmountMinor.Valid {
		old := row.OldAmountMinor.Int64
		change.OldAmountMinor = &old
	}

	return change
}
