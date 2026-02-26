package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"boring-budget/internal/domain"
	queries "boring-budget/internal/store/sqlite/sqlc"
)

type SavingsRepo struct {
	db      *sql.DB
	queries *queries.Queries
}

func NewSavingsRepo(db *sql.DB) *SavingsRepo {
	return &SavingsRepo{
		db:      db,
		queries: queries.New(db),
	}
}

func (r *SavingsRepo) AddEvent(ctx context.Context, input domain.SavingsEventAddInput) (domain.SavingsEvent, error) {
	if r.db == nil {
		return domain.SavingsEvent{}, fmt.Errorf("add savings event: db is nil")
	}

	note := sql.NullString{}
	if strings.TrimSpace(input.Note) != "" {
		note = sql.NullString{String: strings.TrimSpace(input.Note), Valid: true}
	}

	result, err := r.queries.CreateSavingsEvent(ctx, queries.CreateSavingsEventParams{
		EventType:    input.EventType,
		AmountMinor:  input.AmountMinor,
		CurrencyCode: input.CurrencyCode,
		EventDateUtc: input.EventDateUTC,
		Note:         note,
	})
	if err != nil {
		return domain.SavingsEvent{}, fmt.Errorf("add savings event: %w", err)
	}

	insertedID, err := result.LastInsertId()
	if err != nil {
		return domain.SavingsEvent{}, fmt.Errorf("add savings event read id: %w", err)
	}

	rows, err := r.queries.ListSavingsEvents(ctx, queries.ListSavingsEventsParams{
		DateFromUtc: input.EventDateUTC,
		DateToUtc:   input.EventDateUTC,
		EventType:   input.EventType,
	})
	if err != nil {
		return domain.SavingsEvent{}, fmt.Errorf("add savings event load inserted row: %w", err)
	}

	for _, row := range rows {
		if row.ID == insertedID {
			return mapSQLCSavingsEventToDomain(row), nil
		}
	}

	return domain.SavingsEvent{}, fmt.Errorf("add savings event load inserted row: id %d not found", insertedID)
}

func (r *SavingsRepo) ListEvents(ctx context.Context, filter domain.SavingsEventListFilter) ([]domain.SavingsEvent, error) {
	if r.db == nil {
		return nil, fmt.Errorf("list savings events: db is nil")
	}

	rows, err := r.queries.ListSavingsEvents(ctx, queries.ListSavingsEventsParams{
		DateFromUtc: nullableString(filter.DateFromUTC),
		DateToUtc:   nullableString(filter.DateToUTC),
		EventType:   nullableString(filter.EventType),
	})
	if err != nil {
		return nil, fmt.Errorf("list savings events: %w", err)
	}

	events := make([]domain.SavingsEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, mapSQLCSavingsEventToDomain(row))
	}
	return events, nil
}

func mapSQLCSavingsEventToDomain(row queries.SavingsEvent) domain.SavingsEvent {
	event := domain.SavingsEvent{
		ID:           row.ID,
		EventType:    row.EventType,
		AmountMinor:  row.AmountMinor,
		CurrencyCode: row.CurrencyCode,
		EventDateUTC: row.EventDateUtc,
		CreatedAtUTC: row.CreatedAtUtc,
	}

	if row.Note.Valid {
		event.Note = row.Note.String
	}

	return event
}
