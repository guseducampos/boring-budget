package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"budgetto/internal/domain"
)

type CategoryRepo struct {
	db *sql.DB
}

func NewCategoryRepo(db *sql.DB) *CategoryRepo {
	return &CategoryRepo{db: db}
}

func (r *CategoryRepo) Add(ctx context.Context, name string) (domain.Category, error) {
	if r.db == nil {
		return domain.Category{}, fmt.Errorf("add category: db is nil")
	}

	result, err := r.db.ExecContext(
		ctx,
		`INSERT INTO categories (name) VALUES (?);`,
		name,
	)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return domain.Category{}, domain.ErrCategoryNameConflict
		}
		return domain.Category{}, fmt.Errorf("add category: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return domain.Category{}, fmt.Errorf("add category read id: %w", err)
	}

	category, err := r.findActiveByID(ctx, id)
	if err != nil {
		return domain.Category{}, err
	}

	return category, nil
}

func (r *CategoryRepo) List(ctx context.Context) ([]domain.Category, error) {
	if r.db == nil {
		return nil, fmt.Errorf("list categories: db is nil")
	}

	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, name, created_at_utc, updated_at_utc
		FROM categories
		WHERE deleted_at_utc IS NULL
		ORDER BY lower(name), id;`,
	)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	categories := make([]domain.Category, 0)
	for rows.Next() {
		var category domain.Category
		if err := rows.Scan(&category.ID, &category.Name, &category.CreatedAtUTC, &category.UpdatedAtUTC); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		categories = append(categories, category)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list categories rows: %w", err)
	}

	return categories, nil
}

func (r *CategoryRepo) Rename(ctx context.Context, id int64, newName string) (domain.Category, error) {
	if r.db == nil {
		return domain.Category{}, fmt.Errorf("rename category: db is nil")
	}

	nowUTC := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE categories
		SET name = ?, updated_at_utc = ?
		WHERE id = ? AND deleted_at_utc IS NULL;`,
		newName,
		nowUTC,
		id,
	)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return domain.Category{}, domain.ErrCategoryNameConflict
		}
		return domain.Category{}, fmt.Errorf("rename category: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return domain.Category{}, fmt.Errorf("rename category rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.Category{}, domain.ErrCategoryNotFound
	}

	category, err := r.findActiveByID(ctx, id)
	if err != nil {
		return domain.Category{}, err
	}

	return category, nil
}

func (r *CategoryRepo) SoftDelete(ctx context.Context, id int64) (domain.CategoryDeleteResult, error) {
	if r.db == nil {
		return domain.CategoryDeleteResult{}, fmt.Errorf("delete category: db is nil")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.CategoryDeleteResult{}, fmt.Errorf("delete category begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	nowUTC := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(
		ctx,
		`UPDATE categories
		SET deleted_at_utc = ?, updated_at_utc = ?
		WHERE id = ? AND deleted_at_utc IS NULL;`,
		nowUTC,
		nowUTC,
		id,
	)
	if err != nil {
		return domain.CategoryDeleteResult{}, fmt.Errorf("delete category mark deleted: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return domain.CategoryDeleteResult{}, fmt.Errorf("delete category rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.CategoryDeleteResult{}, domain.ErrCategoryNotFound
	}

	orphanResult, err := tx.ExecContext(
		ctx,
		`UPDATE transactions
		SET category_id = NULL, updated_at_utc = ?
		WHERE category_id = ? AND deleted_at_utc IS NULL;`,
		nowUTC,
		id,
	)
	if err != nil {
		return domain.CategoryDeleteResult{}, fmt.Errorf("delete category orphan transactions: %w", err)
	}

	orphanedCount, err := orphanResult.RowsAffected()
	if err != nil {
		return domain.CategoryDeleteResult{}, fmt.Errorf("delete category orphan rows affected: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return domain.CategoryDeleteResult{}, fmt.Errorf("delete category commit: %w", err)
	}

	return domain.CategoryDeleteResult{
		CategoryID:           id,
		DeletedAtUTC:         nowUTC,
		OrphanedTransactions: orphanedCount,
	}, nil
}

func (r *CategoryRepo) findActiveByID(ctx context.Context, id int64) (domain.Category, error) {
	var category domain.Category

	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, name, created_at_utc, updated_at_utc
		FROM categories
		WHERE id = ? AND deleted_at_utc IS NULL;`,
		id,
	).Scan(&category.ID, &category.Name, &category.CreatedAtUTC, &category.UpdatedAtUTC)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Category{}, domain.ErrCategoryNotFound
		}
		return domain.Category{}, fmt.Errorf("find category by id: %w", err)
	}

	return category, nil
}

func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") || strings.Contains(message, "constraint failed")
}
