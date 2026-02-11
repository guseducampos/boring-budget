package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"boring-budget/internal/domain"
	queries "boring-budget/internal/store/sqlite/sqlc"
)

type FXRepo struct {
	db      *sql.DB
	queries *queries.Queries
}

func NewFXRepo(db *sql.DB) *FXRepo {
	return &FXRepo{
		db:      db,
		queries: queries.New(db),
	}
}

func (r *FXRepo) GetSnapshotByKey(ctx context.Context, provider, baseCurrency, quoteCurrency, rateDate string, isEstimate bool) (domain.FXRateSnapshot, error) {
	if r.db == nil {
		return domain.FXRateSnapshot{}, fmt.Errorf("get fx snapshot: db is nil")
	}

	row, err := r.queries.GetFXRateSnapshotByKey(ctx, queries.GetFXRateSnapshotByKeyParams{
		Provider:      strings.TrimSpace(provider),
		BaseCurrency:  strings.TrimSpace(baseCurrency),
		QuoteCurrency: strings.TrimSpace(quoteCurrency),
		RateDate:      strings.TrimSpace(rateDate),
		IsEstimate:    boolToInt64(isEstimate),
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.FXRateSnapshot{}, domain.ErrFXRateUnavailable
		}
		return domain.FXRateSnapshot{}, fmt.Errorf("get fx snapshot by key: %w", err)
	}

	return mapSQLCFXRateSnapshotToDomain(row), nil
}

func (r *FXRepo) CreateSnapshot(ctx context.Context, input domain.FXRateSnapshotCreateInput) (domain.FXRateSnapshot, error) {
	if r.db == nil {
		return domain.FXRateSnapshot{}, fmt.Errorf("create fx snapshot: db is nil")
	}

	if err := domain.ValidateFXRate(input.Rate); err != nil {
		return domain.FXRateSnapshot{}, err
	}

	result, err := r.queries.CreateFXRateSnapshot(ctx, queries.CreateFXRateSnapshotParams{
		Provider:      strings.TrimSpace(input.Provider),
		BaseCurrency:  strings.TrimSpace(input.BaseCurrency),
		QuoteCurrency: strings.TrimSpace(input.QuoteCurrency),
		Rate:          strings.TrimSpace(input.Rate),
		RateDate:      strings.TrimSpace(input.RateDate),
		IsEstimate:    boolToInt64(input.IsEstimate),
		FetchedAtUtc:  strings.TrimSpace(input.FetchedAtUTC),
	})
	if err != nil {
		return domain.FXRateSnapshot{}, fmt.Errorf("create fx snapshot: %w", err)
	}

	insertedID, err := result.LastInsertId()
	if err != nil {
		return domain.FXRateSnapshot{}, fmt.Errorf("create fx snapshot last insert id: %w", err)
	}

	return domain.FXRateSnapshot{
		ID:            insertedID,
		Provider:      strings.TrimSpace(input.Provider),
		BaseCurrency:  strings.TrimSpace(input.BaseCurrency),
		QuoteCurrency: strings.TrimSpace(input.QuoteCurrency),
		Rate:          strings.TrimSpace(input.Rate),
		RateDate:      strings.TrimSpace(input.RateDate),
		IsEstimate:    input.IsEstimate,
		FetchedAtUTC:  strings.TrimSpace(input.FetchedAtUTC),
	}, nil
}

func mapSQLCFXRateSnapshotToDomain(row queries.FxRateSnapshot) domain.FXRateSnapshot {
	return domain.FXRateSnapshot{
		ID:            row.ID,
		Provider:      row.Provider,
		BaseCurrency:  row.BaseCurrency,
		QuoteCurrency: row.QuoteCurrency,
		Rate:          row.Rate,
		RateDate:      row.RateDate,
		IsEstimate:    int64ToBool(row.IsEstimate),
		FetchedAtUTC:  row.FetchedAtUtc,
	}
}

func boolToInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func int64ToBool(value int64) bool {
	return value != 0
}
