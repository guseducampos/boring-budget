package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"budgetto/internal/domain"
)

func TestCategoryRepoAddAndList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openCategoryTestDB(t)
	defer db.Close()

	repo := NewCategoryRepo(db)
	if _, err := repo.Add(ctx, "Rent"); err != nil {
		t.Fatalf("add rent: %v", err)
	}
	if _, err := repo.Add(ctx, "food"); err != nil {
		t.Fatalf("add food: %v", err)
	}

	categories, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list categories: %v", err)
	}

	if len(categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(categories))
	}
	if categories[0].Name != "food" || categories[1].Name != "Rent" {
		t.Fatalf("unexpected list order: %+v", categories)
	}
	if categories[0].CreatedAtUTC == "" || categories[0].UpdatedAtUTC == "" {
		t.Fatalf("expected timestamps to be populated: %+v", categories[0])
	}
}

func TestCategoryRepoAddDuplicateNameReturnsConflict(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openCategoryTestDB(t)
	defer db.Close()

	repo := NewCategoryRepo(db)
	if _, err := repo.Add(ctx, "Transport"); err != nil {
		t.Fatalf("add transport: %v", err)
	}

	_, err := repo.Add(ctx, "transport")
	if !errors.Is(err, domain.ErrCategoryNameConflict) {
		t.Fatalf("expected ErrCategoryNameConflict, got %v", err)
	}
}

func TestCategoryRepoRenameNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openCategoryTestDB(t)
	defer db.Close()

	repo := NewCategoryRepo(db)
	_, err := repo.Rename(ctx, 9999, "Utilities")
	if !errors.Is(err, domain.ErrCategoryNotFound) {
		t.Fatalf("expected ErrCategoryNotFound, got %v", err)
	}
}

func TestCategoryRepoSoftDeleteOrphansActiveTransactionsOnly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openCategoryTestDB(t)
	defer db.Close()

	repo := NewCategoryRepo(db)
	category, err := repo.Add(ctx, "Bills")
	if err != nil {
		t.Fatalf("add category: %v", err)
	}

	activeTxID := insertTransaction(t, ctx, db, category.ID, "")
	deletedTxID := insertTransaction(t, ctx, db, category.ID, time.Now().UTC().Format(time.RFC3339Nano))

	deleteResult, err := repo.SoftDelete(ctx, category.ID)
	if err != nil {
		t.Fatalf("soft delete category: %v", err)
	}
	if deleteResult.OrphanedTransactions != 1 {
		t.Fatalf("expected 1 orphaned transaction, got %d", deleteResult.OrphanedTransactions)
	}

	var categoryDeletedAt sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT deleted_at_utc FROM categories WHERE id = ?;", category.ID).Scan(&categoryDeletedAt); err != nil {
		t.Fatalf("read category deleted_at_utc: %v", err)
	}
	if !categoryDeletedAt.Valid || categoryDeletedAt.String == "" {
		t.Fatalf("expected category deleted_at_utc to be set")
	}

	activeCategoryID := readTransactionCategoryID(t, ctx, db, activeTxID)
	if activeCategoryID.Valid {
		t.Fatalf("expected active transaction to be orphaned, got category_id=%v", activeCategoryID.Int64)
	}

	deletedCategoryID := readTransactionCategoryID(t, ctx, db, deletedTxID)
	if !deletedCategoryID.Valid || deletedCategoryID.Int64 != category.ID {
		t.Fatalf("expected soft-deleted transaction to keep category_id=%d, got %+v", category.ID, deletedCategoryID)
	}

	categories, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list categories: %v", err)
	}
	if len(categories) != 0 {
		t.Fatalf("expected deleted category to be excluded from list, got %d rows", len(categories))
	}
}

func openCategoryTestDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "category-test.db")
	db, err := OpenAndMigrate(ctx, dbPath, migrationsDirFromThisFile(t))
	if err != nil {
		t.Fatalf("open and migrate test db: %v", err)
	}
	return db
}

func migrationsDirFromThisFile(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve current file path")
	}

	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	return filepath.Join(projectRoot, "migrations")
}

func insertTransaction(t *testing.T, ctx context.Context, db *sql.DB, categoryID int64, deletedAtUTC string) int64 {
	t.Helper()

	result, err := db.ExecContext(
		ctx,
		`INSERT INTO transactions (
			type,
			amount_minor,
			currency_code,
			transaction_date_utc,
			category_id,
			note,
			deleted_at_utc
		) VALUES (?, ?, ?, ?, ?, ?, ?);`,
		"expense",
		1000,
		"USD",
		time.Now().UTC().Format(time.RFC3339Nano),
		categoryID,
		"test transaction",
		nullIfEmpty(deletedAtUTC),
	)
	if err != nil {
		t.Fatalf("insert transaction: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read inserted transaction id: %v", err)
	}

	return id
}

func readTransactionCategoryID(t *testing.T, ctx context.Context, db *sql.DB, transactionID int64) sql.NullInt64 {
	t.Helper()

	var categoryID sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT category_id FROM transactions WHERE id = ?;", transactionID).Scan(&categoryID); err != nil {
		t.Fatalf("read transaction category_id: %v", err)
	}
	return categoryID
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
