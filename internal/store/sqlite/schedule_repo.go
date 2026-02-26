package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"boring-budget/internal/domain"
	queries "boring-budget/internal/store/sqlite/sqlc"
)

type ScheduleRepo struct {
	db      *sql.DB
	queries *queries.Queries
}

func NewScheduleRepo(db *sql.DB) *ScheduleRepo {
	return &ScheduleRepo{
		db:      db,
		queries: queries.New(db),
	}
}

func (r *ScheduleRepo) Add(ctx context.Context, input domain.ScheduledPaymentAddInput) (domain.ScheduledPayment, error) {
	if r.db == nil {
		return domain.ScheduledPayment{}, fmt.Errorf("add schedule: db is nil")
	}

	categoryID := sql.NullInt64{}
	if input.CategoryID != nil {
		exists, err := r.queries.ExistsActiveCategoryByID(ctx, *input.CategoryID)
		if err != nil {
			return domain.ScheduledPayment{}, fmt.Errorf("add schedule check category: %w", err)
		}
		if !isTruthy(exists) {
			return domain.ScheduledPayment{}, domain.ErrCategoryNotFound
		}
		categoryID = sql.NullInt64{Int64: *input.CategoryID, Valid: true}
	}

	endMonthKey := sql.NullString{}
	if input.EndMonthKey != nil {
		endMonthKey = sql.NullString{String: *input.EndMonthKey, Valid: true}
	}

	nowUTC := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.queries.CreateScheduledPayment(ctx, queries.CreateScheduledPaymentParams{
		Name:          input.Name,
		AmountMinor:   input.AmountMinor,
		CurrencyCode:  input.CurrencyCode,
		DayOfMonth:    int64(input.DayOfMonth),
		StartMonthKey: input.StartMonthKey,
		EndMonthKey:   endMonthKey,
		CategoryID:    categoryID,
		Note:          nullableString(input.Note),
		UpdatedAtUtc:  nowUTC,
	})
	if err != nil {
		return domain.ScheduledPayment{}, fmt.Errorf("add schedule insert: %w", err)
	}

	scheduleID, err := result.LastInsertId()
	if err != nil {
		return domain.ScheduledPayment{}, fmt.Errorf("add schedule read id: %w", err)
	}

	return r.GetActiveByID(ctx, scheduleID)
}

func (r *ScheduleRepo) List(ctx context.Context, includeDeleted bool) ([]domain.ScheduledPayment, error) {
	if r.db == nil {
		return nil, fmt.Errorf("list schedules: db is nil")
	}

	includeDeletedInt := int64(0)
	if includeDeleted {
		includeDeletedInt = 1
	}

	rows, err := r.queries.ListScheduledPayments(ctx, includeDeletedInt)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}

	schedules := make([]domain.ScheduledPayment, 0, len(rows))
	for _, row := range rows {
		schedules = append(schedules, mapSQLCScheduleListRowToDomain(row))
	}

	return schedules, nil
}

func (r *ScheduleRepo) GetActiveByID(ctx context.Context, id int64) (domain.ScheduledPayment, error) {
	if r.db == nil {
		return domain.ScheduledPayment{}, fmt.Errorf("get schedule: db is nil")
	}

	row, err := r.queries.GetActiveScheduledPaymentByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.ScheduledPayment{}, domain.ErrScheduleNotFound
		}
		return domain.ScheduledPayment{}, fmt.Errorf("get schedule by id: %w", err)
	}

	return mapSQLCScheduledPaymentToDomain(row), nil
}

func (r *ScheduleRepo) Delete(ctx context.Context, id int64) (domain.ScheduledPaymentDeleteResult, error) {
	if r.db == nil {
		return domain.ScheduledPaymentDeleteResult{}, fmt.Errorf("delete schedule: db is nil")
	}

	nowUTC := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.queries.SoftDeleteScheduledPayment(ctx, queries.SoftDeleteScheduledPaymentParams{
		DeletedAtUtc: sql.NullString{String: nowUTC, Valid: true},
		UpdatedAtUtc: nowUTC,
		ID:           id,
	})
	if err != nil {
		return domain.ScheduledPaymentDeleteResult{}, fmt.Errorf("delete schedule: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return domain.ScheduledPaymentDeleteResult{}, fmt.Errorf("delete schedule rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.ScheduledPaymentDeleteResult{}, domain.ErrScheduleNotFound
	}

	return domain.ScheduledPaymentDeleteResult{
		ScheduleID:   id,
		DeletedAtUTC: nowUTC,
	}, nil
}

func (r *ScheduleRepo) ListExecutionsByScheduleID(ctx context.Context, scheduleID int64) ([]domain.ScheduledPaymentExecution, error) {
	if r.db == nil {
		return nil, fmt.Errorf("list schedule executions: db is nil")
	}

	rows, err := r.queries.ListScheduledPaymentExecutionsByScheduleID(ctx, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("list schedule executions: %w", err)
	}

	executions := make([]domain.ScheduledPaymentExecution, 0, len(rows))
	for _, row := range rows {
		executions = append(executions, mapSQLCScheduleExecutionToDomain(row))
	}

	return executions, nil
}

func (r *ScheduleRepo) ClaimExecution(ctx context.Context, scheduleID int64, monthKey, createdAtUTC string) (bool, error) {
	if r.db == nil {
		return false, fmt.Errorf("claim schedule execution: db is nil")
	}

	_, err := r.queries.ClaimScheduledPaymentExecution(ctx, queries.ClaimScheduledPaymentExecutionParams{
		ScheduleID:   scheduleID,
		MonthKey:     monthKey,
		CreatedAtUtc: createdAtUTC,
	})
	if err != nil {
		if isUniqueConstraintErr(err) {
			return false, nil
		}
		return false, fmt.Errorf("claim schedule execution: %w", err)
	}

	return true, nil
}

func (r *ScheduleRepo) AttachExecutionEntry(ctx context.Context, scheduleID int64, monthKey string, entryID int64) error {
	if r.db == nil {
		return fmt.Errorf("attach schedule execution entry: db is nil")
	}

	result, err := r.queries.AttachScheduledPaymentExecutionEntry(ctx, queries.AttachScheduledPaymentExecutionEntryParams{
		EntryID:    sql.NullInt64{Int64: entryID, Valid: true},
		ScheduleID: scheduleID,
		MonthKey:   monthKey,
	})
	if err != nil {
		return fmt.Errorf("attach schedule execution entry: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("attach schedule execution entry rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.ErrScheduleExecutionNotFound
	}

	return nil
}

func mapSQLCScheduledPaymentToDomain(row queries.ScheduledPayment) domain.ScheduledPayment {
	schedule := domain.ScheduledPayment{
		ID:            row.ID,
		Name:          row.Name,
		AmountMinor:   row.AmountMinor,
		CurrencyCode:  row.CurrencyCode,
		DayOfMonth:    int(row.DayOfMonth),
		StartMonthKey: row.StartMonthKey,
		CreatedAtUTC:  row.CreatedAtUtc,
		UpdatedAtUTC:  row.UpdatedAtUtc,
	}

	if row.EndMonthKey.Valid {
		value := row.EndMonthKey.String
		schedule.EndMonthKey = &value
	}
	if row.CategoryID.Valid {
		value := row.CategoryID.Int64
		schedule.CategoryID = &value
	}
	if row.Note.Valid {
		schedule.Note = row.Note.String
	}
	if row.DeletedAtUtc.Valid {
		value := row.DeletedAtUtc.String
		schedule.DeletedAtUTC = &value
	}

	return schedule
}

func mapSQLCScheduleListRowToDomain(row queries.ListScheduledPaymentsRow) domain.ScheduledPayment {
	schedule := domain.ScheduledPayment{
		ID:            row.ID,
		Name:          row.Name,
		AmountMinor:   row.AmountMinor,
		CurrencyCode:  row.CurrencyCode,
		DayOfMonth:    int(row.DayOfMonth),
		StartMonthKey: row.StartMonthKey,
		CreatedAtUTC:  row.CreatedAtUtc,
		UpdatedAtUTC:  row.UpdatedAtUtc,
	}

	if row.EndMonthKey.Valid {
		value := row.EndMonthKey.String
		schedule.EndMonthKey = &value
	}
	if row.CategoryID.Valid {
		value := row.CategoryID.Int64
		schedule.CategoryID = &value
	}
	if row.Note.Valid {
		schedule.Note = row.Note.String
	}
	if row.DeletedAtUtc.Valid {
		value := row.DeletedAtUtc.String
		schedule.DeletedAtUTC = &value
	}

	if value := nullableStringFromAny(row.LastExecutedMonthKey); value != nil {
		schedule.LastExecutedMonthKey = value
	}

	return schedule
}

func mapSQLCScheduleExecutionToDomain(row queries.ScheduledPaymentExecution) domain.ScheduledPaymentExecution {
	execution := domain.ScheduledPaymentExecution{
		ID:           row.ID,
		ScheduleID:   row.ScheduleID,
		MonthKey:     row.MonthKey,
		CreatedAtUTC: row.CreatedAtUtc,
	}

	if row.EntryID.Valid {
		value := row.EntryID.Int64
		execution.EntryID = &value
	}

	return execution
}

func nullableStringFromAny(value any) *string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		normalized := strings.TrimSpace(v)
		if normalized == "" {
			return nil
		}
		return &normalized
	case []byte:
		normalized := strings.TrimSpace(string(v))
		if normalized == "" {
			return nil
		}
		return &normalized
	default:
		normalized := strings.TrimSpace(fmt.Sprintf("%v", v))
		if normalized == "" || normalized == "<nil>" {
			return nil
		}
		return &normalized
	}
}
