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

func TestLabelRepoCRUDAndSoftDeleteLinks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newMigratedTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	repo, err := NewLabelRepo(db)
	if err != nil {
		t.Fatalf("new label repo: %v", err)
	}

	first, err := repo.Add(ctx, "  groceries ")
	if err != nil {
		t.Fatalf("add first label: %v", err)
	}
	if first.Name != "groceries" {
		t.Fatalf("expected trimmed label name, got %q", first.Name)
	}

	second, err := repo.Add(ctx, "travel")
	if err != nil {
		t.Fatalf("add second label: %v", err)
	}

	labels, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list labels: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}
	if labels[0].ID != first.ID || labels[1].ID != second.ID {
		t.Fatalf("expected deterministic ordering by name/id")
	}

	renamed, err := repo.Rename(ctx, first.ID, "food")
	if err != nil {
		t.Fatalf("rename label: %v", err)
	}
	if renamed.Name != "food" {
		t.Fatalf("expected renamed label to be food, got %q", renamed.Name)
	}

	txnID := insertTestTransaction(t, ctx, db)
	linkID := insertTestTransactionLabelLink(t, ctx, db, txnID, first.ID)

	deleteResult, err := repo.Delete(ctx, first.ID)
	if err != nil {
		t.Fatalf("delete label: %v", err)
	}
	if deleteResult.LabelID != first.ID {
		t.Fatalf("expected deleted label id %d, got %d", first.ID, deleteResult.LabelID)
	}
	if deleteResult.DetachedLinks != 1 {
		t.Fatalf("expected 1 detached link, got %d", deleteResult.DetachedLinks)
	}

	assertLabelDeleted(t, ctx, db, first.ID)
	assertTransactionLabelLinkDeleted(t, ctx, db, linkID)
	assertTransactionStillActive(t, ctx, db, txnID)

	labels, err = repo.List(ctx)
	if err != nil {
		t.Fatalf("list labels after delete: %v", err)
	}
	if len(labels) != 1 || labels[0].ID != second.ID {
		t.Fatalf("expected only second label to remain active")
	}
}

func TestLabelRepoConflictAndNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newMigratedTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	repo, err := NewLabelRepo(db)
	if err != nil {
		t.Fatalf("new label repo: %v", err)
	}

	food, err := repo.Add(ctx, "food")
	if err != nil {
		t.Fatalf("add food: %v", err)
	}

	if _, err := repo.Add(ctx, "FOOD"); !errors.Is(err, domain.ErrLabelNameConflict) {
		t.Fatalf("expected ErrLabelNameConflict on add, got %v", err)
	}

	travel, err := repo.Add(ctx, "travel")
	if err != nil {
		t.Fatalf("add travel: %v", err)
	}

	if _, err := repo.Rename(ctx, travel.ID, food.Name); !errors.Is(err, domain.ErrLabelNameConflict) {
		t.Fatalf("expected ErrLabelNameConflict on rename, got %v", err)
	}

	if _, err := repo.Rename(ctx, 999999, "other"); !errors.Is(err, domain.ErrLabelNotFound) {
		t.Fatalf("expected ErrLabelNotFound on rename, got %v", err)
	}

	if _, err := repo.Delete(ctx, 999999); !errors.Is(err, domain.ErrLabelNotFound) {
		t.Fatalf("expected ErrLabelNotFound on delete, got %v", err)
	}
}

func newMigratedTestDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	migrationsDir := migrationsPath(t)

	db, err := OpenAndMigrate(ctx, dbPath, migrationsDir)
	if err != nil {
		t.Fatalf("open and migrate test db: %v", err)
	}

	return db
}

func migrationsPath(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime caller failed")
	}

	return filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "migrations")
}

func insertTestTransaction(t *testing.T, ctx context.Context, db *sql.DB) int64 {
	t.Helper()

	result, err := db.ExecContext(ctx, `
		INSERT INTO transactions (type, amount_minor, currency_code, transaction_date_utc, note)
		VALUES ('expense', 1299, 'USD', ?, 'label test txn');
	`, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert transaction: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id transaction: %v", err)
	}
	return id
}

func insertTestTransactionLabelLink(t *testing.T, ctx context.Context, db *sql.DB, txnID, labelID int64) int64 {
	t.Helper()

	result, err := db.ExecContext(ctx, `
		INSERT INTO transaction_labels (transaction_id, label_id)
		VALUES (?, ?);
	`, txnID, labelID)
	if err != nil {
		t.Fatalf("insert transaction label link: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id transaction label link: %v", err)
	}

	return id
}

func assertLabelDeleted(t *testing.T, ctx context.Context, db *sql.DB, labelID int64) {
	t.Helper()

	var deletedAt sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT deleted_at_utc FROM labels WHERE id = ?;`, labelID).Scan(&deletedAt); err != nil {
		t.Fatalf("query label deleted_at_utc: %v", err)
	}

	if !deletedAt.Valid || deletedAt.String == "" {
		t.Fatalf("expected label to be soft deleted")
	}
}

func assertTransactionLabelLinkDeleted(t *testing.T, ctx context.Context, db *sql.DB, linkID int64) {
	t.Helper()

	var deletedAt sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT deleted_at_utc FROM transaction_labels WHERE id = ?;`, linkID).Scan(&deletedAt); err != nil {
		t.Fatalf("query transaction_labels.deleted_at_utc: %v", err)
	}

	if !deletedAt.Valid || deletedAt.String == "" {
		t.Fatalf("expected transaction label link to be soft deleted")
	}
}

func assertTransactionStillActive(t *testing.T, ctx context.Context, db *sql.DB, txnID int64) {
	t.Helper()

	var deletedAt sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT deleted_at_utc FROM transactions WHERE id = ?;`, txnID).Scan(&deletedAt); err != nil {
		t.Fatalf("query transactions.deleted_at_utc: %v", err)
	}

	if deletedAt.Valid {
		t.Fatalf("expected transaction to remain active")
	}
}
