package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"budgetto/internal/domain"
	queries "budgetto/internal/store/sqlite/sqlc"
)

type CategoryRepo struct {
	db      *sql.DB
	queries *queries.Queries
}

func NewCategoryRepo(db *sql.DB) *CategoryRepo {
	return &CategoryRepo{
		db:      db,
		queries: queries.New(db),
	}
}

func (r *CategoryRepo) Add(ctx context.Context, name string) (domain.Category, error) {
	if r.db == nil {
		return domain.Category{}, fmt.Errorf("add category: db is nil")
	}

	result, err := r.queries.CreateCategory(ctx, name)
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

	rows, err := r.queries.ListActiveCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}

	categories := make([]domain.Category, 0, len(rows))
	for _, row := range rows {
		categories = append(categories, domain.Category{
			ID:           row.ID,
			Name:         row.Name,
			CreatedAtUTC: row.CreatedAtUtc,
			UpdatedAtUTC: row.UpdatedAtUtc,
		})
	}

	return categories, nil
}

func (r *CategoryRepo) ListByIDs(ctx context.Context, ids []int64) ([]domain.Category, error) {
	if r.db == nil {
		return nil, fmt.Errorf("list categories by ids: db is nil")
	}

	normalizedIDs := normalizeCategoryIDs(ids)
	if len(normalizedIDs) == 0 {
		return []domain.Category{}, nil
	}

	categories := make([]domain.Category, 0, len(normalizedIDs))
	for _, id := range normalizedIDs {
		category, err := r.findActiveByID(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrCategoryNotFound) {
				continue
			}
			return nil, fmt.Errorf("list categories by ids: %w", err)
		}
		categories = append(categories, category)
	}

	return categories, nil
}

func (r *CategoryRepo) Rename(ctx context.Context, id int64, newName string) (domain.Category, error) {
	if r.db == nil {
		return domain.Category{}, fmt.Errorf("rename category: db is nil")
	}

	nowUTC := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.queries.RenameActiveCategory(ctx, queries.RenameActiveCategoryParams{
		Name:         newName,
		UpdatedAtUtc: nowUTC,
		ID:           id,
	})
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
	qtx := r.queries.WithTx(tx)
	result, err := qtx.SoftDeleteCategory(ctx, queries.SoftDeleteCategoryParams{
		DeletedAtUtc: sql.NullString{String: nowUTC, Valid: true},
		UpdatedAtUtc: nowUTC,
		ID:           id,
	})
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

	orphanResult, err := qtx.OrphanActiveTransactionsByCategoryID(ctx, queries.OrphanActiveTransactionsByCategoryIDParams{
		UpdatedAtUtc: nowUTC,
		CategoryID:   sql.NullInt64{Int64: id, Valid: true},
	})
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
	row, err := r.queries.GetActiveCategoryByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Category{}, domain.ErrCategoryNotFound
		}
		return domain.Category{}, fmt.Errorf("find category by id: %w", err)
	}

	return domain.Category{
		ID:           row.ID,
		Name:         row.Name,
		CreatedAtUTC: row.CreatedAtUtc,
		UpdatedAtUTC: row.UpdatedAtUtc,
	}, nil
}

func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") || strings.Contains(message, "constraint failed")
}

func normalizeCategoryIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}

	unique := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		unique[id] = struct{}{}
	}
	if len(unique) == 0 {
		return nil
	}

	normalized := make([]int64, 0, len(unique))
	for id := range unique {
		normalized = append(normalized, id)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i] < normalized[j]
	})

	return normalized
}
