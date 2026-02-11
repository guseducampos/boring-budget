package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"boring-budget/internal/domain"
	"boring-budget/internal/ports"
	queries "boring-budget/internal/store/sqlite/sqlc"
)

type EntryRepo struct {
	db      *sql.DB
	queries *queries.Queries
	tx      *sql.Tx
}

var _ ports.EntryRepositoryTxBinder = (*EntryRepo)(nil)

func NewEntryRepo(db *sql.DB) *EntryRepo {
	return &EntryRepo{
		db:      db,
		queries: queries.New(db),
	}
}

func (r *EntryRepo) BindTx(tx *sql.Tx) ports.EntryRepository {
	if tx == nil {
		return r
	}

	return &EntryRepo{
		db:      r.db,
		queries: r.queries.WithTx(tx),
		tx:      tx,
	}
}

func (r *EntryRepo) Add(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error) {
	tx, qtx, ownsTx, err := r.writeQueries(ctx, "add entry")
	if err != nil {
		return domain.Entry{}, err
	}
	if ownsTx {
		defer func() {
			_ = tx.Rollback()
		}()
	}

	categoryID := sql.NullInt64{}
	if input.CategoryID != nil {
		isActive, err := qtx.ExistsActiveCategoryByID(ctx, *input.CategoryID)
		if err != nil {
			return domain.Entry{}, fmt.Errorf("add entry check category: %w", err)
		}
		if !isTruthy(isActive) {
			return domain.Entry{}, domain.ErrCategoryNotFound
		}
		categoryID = sql.NullInt64{Int64: *input.CategoryID, Valid: true}
	}

	note := sql.NullString{}
	if strings.TrimSpace(input.Note) != "" {
		note = sql.NullString{String: input.Note, Valid: true}
	}

	result, err := qtx.CreateEntry(ctx, queries.CreateEntryParams{
		Type:               input.Type,
		AmountMinor:        input.AmountMinor,
		CurrencyCode:       input.CurrencyCode,
		TransactionDateUtc: input.TransactionDateUTC,
		CategoryID:         categoryID,
		Note:               note,
	})
	if err != nil {
		return domain.Entry{}, fmt.Errorf("add entry insert: %w", err)
	}

	entryID, err := result.LastInsertId()
	if err != nil {
		return domain.Entry{}, fmt.Errorf("add entry read id: %w", err)
	}

	for _, labelID := range input.LabelIDs {
		isActive, err := qtx.ExistsActiveLabelByID(ctx, labelID)
		if err != nil {
			return domain.Entry{}, fmt.Errorf("add entry check label %d: %w", labelID, err)
		}
		if !isTruthy(isActive) {
			return domain.Entry{}, domain.ErrLabelNotFound
		}

		if _, err := qtx.AddEntryLabelLink(ctx, queries.AddEntryLabelLinkParams{
			TransactionID: entryID,
			LabelID:       labelID,
		}); err != nil {
			return domain.Entry{}, fmt.Errorf("add entry label link %d: %w", labelID, err)
		}
	}

	if ownsTx {
		if err := tx.Commit(); err != nil {
			return domain.Entry{}, fmt.Errorf("add entry commit: %w", err)
		}
	}

	entry, err := r.getActiveByID(ctx, entryID)
	if err != nil {
		return domain.Entry{}, err
	}
	return entry, nil
}

func (r *EntryRepo) Update(ctx context.Context, input domain.EntryUpdateInput) (domain.Entry, error) {
	tx, qtx, ownsTx, err := r.writeQueries(ctx, "update entry")
	if err != nil {
		return domain.Entry{}, err
	}
	if ownsTx {
		defer func() {
			_ = tx.Rollback()
		}()
	}

	current, err := qtx.GetActiveEntryByID(ctx, input.ID)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Entry{}, domain.ErrEntryNotFound
		}
		return domain.Entry{}, fmt.Errorf("update entry load current: %w", err)
	}

	categoryID := current.CategoryID
	clearCategory := int64(0)
	setCategoryID := int64(0)
	if input.SetCategory {
		if input.CategoryID == nil {
			clearCategory = 1
			categoryID = sql.NullInt64{}
		} else {
			isActive, err := qtx.ExistsActiveCategoryByID(ctx, *input.CategoryID)
			if err != nil {
				return domain.Entry{}, fmt.Errorf("update entry check category: %w", err)
			}
			if !isTruthy(isActive) {
				return domain.Entry{}, domain.ErrCategoryNotFound
			}
			setCategoryID = 1
			categoryID = sql.NullInt64{Int64: *input.CategoryID, Valid: true}
		}
	}

	setType := int64(0)
	entryType := current.Type
	if input.Type != nil {
		setType = 1
		entryType = *input.Type
	}

	setAmountMinor := int64(0)
	amountMinor := current.AmountMinor
	if input.AmountMinor != nil {
		setAmountMinor = 1
		amountMinor = *input.AmountMinor
	}

	setCurrencyCode := int64(0)
	currencyCode := current.CurrencyCode
	if input.CurrencyCode != nil {
		setCurrencyCode = 1
		currencyCode = *input.CurrencyCode
	}

	setTransactionDateUTC := int64(0)
	transactionDateUTC := current.TransactionDateUtc
	if input.TransactionDateUTC != nil {
		setTransactionDateUTC = 1
		transactionDateUTC = *input.TransactionDateUTC
	}

	clearNote := int64(0)
	setNote := int64(0)
	note := current.Note
	if input.SetNote {
		if input.Note == nil {
			clearNote = 1
			note = sql.NullString{}
		} else {
			setNote = 1
			note = sql.NullString{String: strings.TrimSpace(*input.Note), Valid: true}
		}
	}

	updatedAtUTC := time.Now().UTC().Format(time.RFC3339Nano)
	updateResult, err := qtx.UpdateEntryByID(ctx, queries.UpdateEntryByIDParams{
		SetType:               setType,
		Type:                  entryType,
		SetAmountMinor:        setAmountMinor,
		AmountMinor:           amountMinor,
		SetCurrencyCode:       setCurrencyCode,
		CurrencyCode:          currencyCode,
		SetTransactionDateUtc: setTransactionDateUTC,
		TransactionDateUtc:    transactionDateUTC,
		ClearCategory:         clearCategory,
		SetCategoryID:         setCategoryID,
		CategoryID:            categoryID,
		ClearNote:             clearNote,
		SetNote:               setNote,
		Note:                  note,
		UpdatedAtUtc:          updatedAtUTC,
		ID:                    input.ID,
	})
	if err != nil {
		return domain.Entry{}, fmt.Errorf("update entry run update: %w", err)
	}

	rowsAffected, err := updateResult.RowsAffected()
	if err != nil {
		return domain.Entry{}, fmt.Errorf("update entry rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.Entry{}, domain.ErrEntryNotFound
	}

	if input.SetLabelIDs {
		for _, labelID := range input.LabelIDs {
			isActive, err := qtx.ExistsActiveLabelByID(ctx, labelID)
			if err != nil {
				return domain.Entry{}, fmt.Errorf("update entry check label %d: %w", labelID, err)
			}
			if !isTruthy(isActive) {
				return domain.Entry{}, domain.ErrLabelNotFound
			}
		}

		_, err := qtx.SoftDeleteEntryLabelLinks(ctx, queries.SoftDeleteEntryLabelLinksParams{
			DeletedAtUtc:  sql.NullString{String: updatedAtUTC, Valid: true},
			TransactionID: input.ID,
		})
		if err != nil {
			return domain.Entry{}, fmt.Errorf("update entry clear label links: %w", err)
		}

		for _, labelID := range input.LabelIDs {
			if _, err := qtx.AddEntryLabelLink(ctx, queries.AddEntryLabelLinkParams{
				TransactionID: input.ID,
				LabelID:       labelID,
			}); err != nil {
				return domain.Entry{}, fmt.Errorf("update entry label link %d: %w", labelID, err)
			}
		}
	}

	if ownsTx {
		if err := tx.Commit(); err != nil {
			return domain.Entry{}, fmt.Errorf("update entry commit: %w", err)
		}
	}

	return r.getActiveByID(ctx, input.ID)
}

func (r *EntryRepo) List(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
	if r.db == nil {
		return nil, fmt.Errorf("list entries: db is nil")
	}

	params := queries.ListActiveEntriesParams{
		EntryType:    nullableString(filter.Type),
		CategoryID:   nullableInt64(filter.CategoryID),
		DateFromUtc:  nullableString(filter.DateFromUTC),
		DateToUtc:    nullableString(filter.DateToUTC),
		NoteContains: nullableString(filter.NoteContains),
	}
	rows, err := r.queries.ListActiveEntries(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list entries: %w", err)
	}

	labelRows, err := r.queries.ListActiveEntryLabelIDsForListFilter(ctx, queries.ListActiveEntryLabelIDsForListFilterParams{
		EntryType:    params.EntryType,
		CategoryID:   params.CategoryID,
		DateFromUtc:  params.DateFromUtc,
		DateToUtc:    params.DateToUtc,
		NoteContains: params.NoteContains,
	})
	if err != nil {
		return nil, fmt.Errorf("list entry labels: %w", err)
	}

	labelIDsByTransactionID := make(map[int64][]int64, len(rows))
	for _, labelRow := range labelRows {
		labelIDsByTransactionID[labelRow.TransactionID] = append(labelIDsByTransactionID[labelRow.TransactionID], labelRow.LabelID)
	}

	entries := make([]domain.Entry, 0, len(rows))
	for _, row := range rows {
		labelIDs := labelIDsByTransactionID[row.ID]
		if labelIDs == nil {
			labelIDs = make([]int64, 0)
		}
		entries = append(entries, mapSQLCTransactionToDomainEntry(row, labelIDs))
	}

	return entries, nil
}

func (r *EntryRepo) Delete(ctx context.Context, id int64) (domain.EntryDeleteResult, error) {
	tx, qtx, ownsTx, err := r.writeQueries(ctx, "delete entry")
	if err != nil {
		return domain.EntryDeleteResult{}, err
	}
	if ownsTx {
		defer func() {
			_ = tx.Rollback()
		}()
	}

	deletedAtUTC := time.Now().UTC().Format(time.RFC3339Nano)

	deleteResult, err := qtx.SoftDeleteEntry(ctx, queries.SoftDeleteEntryParams{
		DeletedAtUtc: sql.NullString{String: deletedAtUTC, Valid: true},
		UpdatedAtUtc: deletedAtUTC,
		ID:           id,
	})
	if err != nil {
		return domain.EntryDeleteResult{}, fmt.Errorf("delete entry mark deleted: %w", err)
	}

	rowsAffected, err := deleteResult.RowsAffected()
	if err != nil {
		return domain.EntryDeleteResult{}, fmt.Errorf("delete entry rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.EntryDeleteResult{}, domain.ErrEntryNotFound
	}

	linksResult, err := qtx.SoftDeleteEntryLabelLinks(ctx, queries.SoftDeleteEntryLabelLinksParams{
		DeletedAtUtc:  sql.NullString{String: deletedAtUTC, Valid: true},
		TransactionID: id,
	})
	if err != nil {
		return domain.EntryDeleteResult{}, fmt.Errorf("delete entry label links: %w", err)
	}

	detachedLabels, err := linksResult.RowsAffected()
	if err != nil {
		return domain.EntryDeleteResult{}, fmt.Errorf("delete entry label links rows affected: %w", err)
	}

	if ownsTx {
		if err := tx.Commit(); err != nil {
			return domain.EntryDeleteResult{}, fmt.Errorf("delete entry commit: %w", err)
		}
	}

	return domain.EntryDeleteResult{
		EntryID:        id,
		DeletedAtUTC:   deletedAtUTC,
		DetachedLabels: detachedLabels,
	}, nil
}

func (r *EntryRepo) writeQueries(ctx context.Context, operation string) (*sql.Tx, *queries.Queries, bool, error) {
	if r.tx != nil {
		return nil, r.queries, false, nil
	}

	if r.db == nil {
		return nil, nil, false, fmt.Errorf("%s: db is nil", operation)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, false, fmt.Errorf("%s begin tx: %w", operation, err)
	}

	return tx, r.queries.WithTx(tx), true, nil
}

func (r *EntryRepo) getActiveByID(ctx context.Context, id int64) (domain.Entry, error) {
	row, err := r.queries.GetActiveEntryByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Entry{}, domain.ErrEntryNotFound
		}
		return domain.Entry{}, fmt.Errorf("get active entry by id: %w", err)
	}

	labelRows, err := r.queries.ListActiveEntryLabelIDs(ctx, id)
	if err != nil {
		return domain.Entry{}, fmt.Errorf("list labels for entry %d: %w", id, err)
	}

	labelIDs := make([]int64, 0, len(labelRows))
	for _, labelRow := range labelRows {
		labelIDs = append(labelIDs, labelRow.LabelID)
	}

	return mapSQLCTransactionToDomainEntry(row, labelIDs), nil
}

func mapSQLCTransactionToDomainEntry(row queries.Transaction, labelIDs []int64) domain.Entry {
	var categoryID *int64
	if row.CategoryID.Valid {
		categoryID = &row.CategoryID.Int64
	}

	note := ""
	if row.Note.Valid {
		note = row.Note.String
	}

	return domain.Entry{
		ID:                 row.ID,
		Type:               row.Type,
		AmountMinor:        row.AmountMinor,
		CurrencyCode:       row.CurrencyCode,
		TransactionDateUTC: row.TransactionDateUtc,
		CategoryID:         categoryID,
		LabelIDs:           labelIDs,
		Note:               note,
		CreatedAtUTC:       row.CreatedAtUtc,
		UpdatedAtUTC:       row.UpdatedAtUtc,
	}
}

func nullableString(value string) sql.NullString {
	if strings.TrimSpace(value) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullableInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func isTruthy(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case int64:
		return v != 0
	case int32:
		return v != 0
	case int:
		return v != 0
	default:
		return false
	}
}
