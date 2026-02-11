package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"budgetto/internal/domain"
)

type LabelRepo struct {
	db *sql.DB
}

func NewLabelRepo(db *sql.DB) (*LabelRepo, error) {
	if db == nil {
		return nil, fmt.Errorf("label repo: db is required")
	}
	return &LabelRepo{db: db}, nil
}

func (r *LabelRepo) Add(ctx context.Context, name string) (domain.Label, error) {
	normalized, err := domain.NormalizeLabelName(name)
	if err != nil {
		return domain.Label{}, err
	}

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO labels (name)
		VALUES (?);
	`, normalized)
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
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, created_at_utc, updated_at_utc, deleted_at_utc
		FROM labels
		WHERE deleted_at_utc IS NULL
		ORDER BY lower(name) ASC, id ASC;
	`)
	if err != nil {
		return nil, wrapStorageErr("list labels", err)
	}
	defer rows.Close()

	labels := make([]domain.Label, 0)
	for rows.Next() {
		label, err := scanLabel(rows)
		if err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}

	if err := rows.Err(); err != nil {
		return nil, wrapStorageErr("iterate labels", err)
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

	_, err = r.db.ExecContext(ctx, `
		UPDATE labels
		SET
			name = ?,
			updated_at_utc = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
		  AND deleted_at_utc IS NULL;
	`, normalized, id)
	if err != nil {
		if isUniqueConstraintError(err) {
			return domain.Label{}, domain.ErrLabelNameConflict
		}
		return domain.Label{}, wrapStorageErr("rename label", err)
	}

	label, err := r.getActiveByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrLabelNotFound) {
			return domain.Label{}, err
		}
		return domain.Label{}, wrapStorageErr("load renamed label", err)
	}

	if !strings.EqualFold(label.Name, normalized) {
		return domain.Label{}, domain.ErrLabelNameConflict
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

	result, err := tx.ExecContext(ctx, `
		UPDATE labels
		SET
			deleted_at_utc = ?,
			updated_at_utc = ?
		WHERE id = ?
		  AND deleted_at_utc IS NULL;
	`, deletedAtUTC, deletedAtUTC, id)
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

	linkResult, err := tx.ExecContext(ctx, `
		UPDATE transaction_labels
		SET deleted_at_utc = ?
		WHERE label_id = ?
		  AND deleted_at_utc IS NULL;
	`, deletedAtUTC, id)
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
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, created_at_utc, updated_at_utc, deleted_at_utc
		FROM labels
		WHERE id = ?
		  AND deleted_at_utc IS NULL;
	`, id)

	label, err := scanLabel(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Label{}, domain.ErrLabelNotFound
		}
		return domain.Label{}, err
	}

	return label, nil
}

type labelScanner interface {
	Scan(dest ...any) error
}

func scanLabel(scanner labelScanner) (domain.Label, error) {
	var (
		label        domain.Label
		createdAtRaw string
		updatedAtRaw string
		deletedAtRaw sql.NullString
	)

	if err := scanner.Scan(&label.ID, &label.Name, &createdAtRaw, &updatedAtRaw, &deletedAtRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Label{}, sql.ErrNoRows
		}
		return domain.Label{}, wrapStorageErr("scan label", err)
	}

	createdAt, err := parseSQLiteTimestamp(createdAtRaw)
	if err != nil {
		return domain.Label{}, wrapStorageErr("parse label created_at_utc", err)
	}
	updatedAt, err := parseSQLiteTimestamp(updatedAtRaw)
	if err != nil {
		return domain.Label{}, wrapStorageErr("parse label updated_at_utc", err)
	}

	label.CreatedAtUTC = createdAt
	label.UpdatedAtUTC = updatedAt

	if deletedAtRaw.Valid {
		deletedAt, err := parseSQLiteTimestamp(deletedAtRaw.String)
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
