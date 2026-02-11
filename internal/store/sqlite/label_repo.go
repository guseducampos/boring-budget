package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"budgetto/internal/domain"
	queries "budgetto/internal/store/sqlite/sqlc"
)

type LabelRepo struct {
	db      *sql.DB
	queries *queries.Queries
}

func NewLabelRepo(db *sql.DB) (*LabelRepo, error) {
	if db == nil {
		return nil, fmt.Errorf("label repo: db is required")
	}
	return &LabelRepo{
		db:      db,
		queries: queries.New(db),
	}, nil
}

func (r *LabelRepo) Add(ctx context.Context, name string) (domain.Label, error) {
	normalized, err := domain.NormalizeLabelName(name)
	if err != nil {
		return domain.Label{}, err
	}

	result, err := r.queries.CreateLabel(ctx, normalized)
	if err != nil {
		if isUniqueConstraintError(err) {
			return domain.Label{}, domain.ErrLabelNameConflict
		}
		return domain.Label{}, wrapStorageErr("insert label", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return domain.Label{}, wrapStorageErr("read inserted label id", err)
	}

	label, err := r.getActiveByID(ctx, id)
	if err != nil {
		return domain.Label{}, err
	}

	return label, nil
}

func (r *LabelRepo) List(ctx context.Context) ([]domain.Label, error) {
	rows, err := r.queries.ListActiveLabels(ctx)
	if err != nil {
		return nil, wrapStorageErr("list labels", err)
	}

	labels := make([]domain.Label, 0, len(rows))
	for _, row := range rows {
		label, err := mapSQLCLabelToDomain(row)
		if err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}

	return labels, nil
}

func (r *LabelRepo) Rename(ctx context.Context, id int64, newName string) (domain.Label, error) {
	if err := domain.ValidateLabelID(id); err != nil {
		return domain.Label{}, err
	}

	normalized, err := domain.NormalizeLabelName(newName)
	if err != nil {
		return domain.Label{}, err
	}

	nowUTC := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.queries.RenameActiveLabel(ctx, queries.RenameActiveLabelParams{
		Name:         normalized,
		UpdatedAtUtc: nowUTC,
		ID:           id,
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			return domain.Label{}, domain.ErrLabelNameConflict
		}
		return domain.Label{}, wrapStorageErr("rename label", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return domain.Label{}, wrapStorageErr("read rename label rows", err)
	}
	if rowsAffected == 0 {
		return domain.Label{}, domain.ErrLabelNotFound
	}

	label, err := r.getActiveByID(ctx, id)
	if err != nil {
		return domain.Label{}, err
	}
	return label, nil
}

func (r *LabelRepo) Delete(ctx context.Context, id int64) (domain.LabelDeleteResult, error) {
	if err := domain.ValidateLabelID(id); err != nil {
		return domain.LabelDeleteResult{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.LabelDeleteResult{}, wrapStorageErr("begin delete label tx", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	deletedAtUTC := time.Now().UTC().Format(time.RFC3339Nano)

	qtx := r.queries.WithTx(tx)

	result, err := qtx.SoftDeleteLabel(ctx, queries.SoftDeleteLabelParams{
		DeletedAtUtc: sql.NullString{String: deletedAtUTC, Valid: true},
		UpdatedAtUtc: deletedAtUTC,
		ID:           id,
	})
	if err != nil {
		return domain.LabelDeleteResult{}, wrapStorageErr("soft delete label", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return domain.LabelDeleteResult{}, wrapStorageErr("read soft delete label rows", err)
	}
	if rowsAffected == 0 {
		return domain.LabelDeleteResult{}, domain.ErrLabelNotFound
	}

	linkResult, err := qtx.SoftDeleteTransactionLabelLinksByLabelID(ctx, queries.SoftDeleteTransactionLabelLinksByLabelIDParams{
		DeletedAtUtc: sql.NullString{String: deletedAtUTC, Valid: true},
		LabelID:      id,
	})
	if err != nil {
		return domain.LabelDeleteResult{}, wrapStorageErr("soft delete transaction_labels links", err)
	}

	detachedLinks, err := linkResult.RowsAffected()
	if err != nil {
		return domain.LabelDeleteResult{}, wrapStorageErr("read transaction_labels rows", err)
	}

	if err := tx.Commit(); err != nil {
		return domain.LabelDeleteResult{}, wrapStorageErr("commit delete label tx", err)
	}

	deletedAt, err := time.Parse(time.RFC3339Nano, deletedAtUTC)
	if err != nil {
		return domain.LabelDeleteResult{}, wrapStorageErr("parse delete timestamp", err)
	}

	return domain.LabelDeleteResult{
		LabelID:       id,
		DetachedLinks: detachedLinks,
		DeletedAtUTC:  deletedAt,
	}, nil
}

func (r *LabelRepo) getActiveByID(ctx context.Context, id int64) (domain.Label, error) {
	row, err := r.queries.GetActiveLabelByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Label{}, domain.ErrLabelNotFound
		}
		return domain.Label{}, wrapStorageErr("load label by id", err)
	}

	return mapSQLCLabelToDomain(row)
}

func mapSQLCLabelToDomain(row queries.Label) (domain.Label, error) {
	createdAt, err := parseSQLiteTimestamp(row.CreatedAtUtc)
	if err != nil {
		return domain.Label{}, wrapStorageErr("parse label created_at_utc", err)
	}
	updatedAt, err := parseSQLiteTimestamp(row.UpdatedAtUtc)
	if err != nil {
		return domain.Label{}, wrapStorageErr("parse label updated_at_utc", err)
	}

	label := domain.Label{
		ID:           row.ID,
		Name:         row.Name,
		CreatedAtUTC: createdAt,
		UpdatedAtUTC: updatedAt,
	}

	if row.DeletedAtUtc.Valid {
		deletedAt, err := parseSQLiteTimestamp(row.DeletedAtUtc.String)
		if err != nil {
			return domain.Label{}, wrapStorageErr("parse label deleted_at_utc", err)
		}
		label.DeletedAtUTC = &deletedAt
	}

	return label, nil
}

func parseSQLiteTimestamp(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return t, nil
	}

	fallback, fallbackErr := time.Parse(time.RFC3339, value)
	if fallbackErr == nil {
		return fallback, nil
	}

	return time.Time{}, fmt.Errorf("invalid sqlite timestamp %q", value)
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") || strings.Contains(msg, "constraint failed: unique")
}

func wrapStorageErr(operation string, err error) error {
	return fmt.Errorf("%w: %s: %v", domain.ErrStorage, operation, err)
}
