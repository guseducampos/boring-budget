package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"budgetto/internal/domain"
	queries "budgetto/internal/store/sqlite/sqlc"
)

type SettingsRepo struct {
	db      *sql.DB
	queries *queries.Queries
}

func NewSettingsRepo(db *sql.DB) *SettingsRepo {
	return &SettingsRepo{
		db:      db,
		queries: queries.New(db),
	}
}

func (r *SettingsRepo) Upsert(ctx context.Context, input domain.SettingsUpsertInput) (domain.Settings, error) {
	if r.db == nil {
		return domain.Settings{}, fmt.Errorf("upsert settings: db is nil")
	}

	normalized, err := domain.NormalizeSettingsInput(input)
	if err != nil {
		return domain.Settings{}, err
	}

	nowUTC := time.Now().UTC().Format(time.RFC3339Nano)
	onboarding := sql.NullString{}
	if normalized.OnboardingCompletedAtUTC != nil {
		onboarding = sql.NullString{String: *normalized.OnboardingCompletedAtUTC, Valid: true}
	}

	if _, err := r.queries.UpsertSettings(ctx, queries.UpsertSettingsParams{
		DefaultCurrencyCode:        normalized.DefaultCurrencyCode,
		DisplayTimezone:            normalized.DisplayTimezone,
		OrphanCountThreshold:       normalized.OrphanCountThreshold,
		OrphanSpendingThresholdBps: normalized.OrphanSpendingThresholdBPS,
		OnboardingCompletedAtUtc:   onboarding,
		UpdatedAtUtc:               nowUTC,
	}); err != nil {
		return domain.Settings{}, fmt.Errorf("upsert settings: %w", err)
	}

	return r.Get(ctx)
}

func (r *SettingsRepo) Get(ctx context.Context) (domain.Settings, error) {
	if r.db == nil {
		return domain.Settings{}, fmt.Errorf("get settings: db is nil")
	}

	row, err := r.queries.GetSettings(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Settings{}, domain.ErrSettingsNotFound
		}
		return domain.Settings{}, fmt.Errorf("get settings: %w", err)
	}

	return mapSQLCSettingsToDomain(row), nil
}

func mapSQLCSettingsToDomain(row queries.Setting) domain.Settings {
	settings := domain.Settings{
		ID:                         row.ID,
		DefaultCurrencyCode:        row.DefaultCurrencyCode,
		DisplayTimezone:            row.DisplayTimezone,
		OrphanCountThreshold:       row.OrphanCountThreshold,
		OrphanSpendingThresholdBPS: row.OrphanSpendingThresholdBps,
		CreatedAtUTC:               row.CreatedAtUtc,
		UpdatedAtUTC:               row.UpdatedAtUtc,
	}

	if row.OnboardingCompletedAtUtc.Valid {
		completedAt := row.OnboardingCompletedAtUtc.String
		settings.OnboardingCompletedAtUTC = &completedAt
	}

	return settings
}
