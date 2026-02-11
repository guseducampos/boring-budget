package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"

	"budgetto/internal/domain"
)

func TestFXRepoCreateAndGetSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openFXTestDB(t)
	defer db.Close()

	repo := NewFXRepo(db)

	created, err := repo.CreateSnapshot(ctx, domain.FXRateSnapshotCreateInput{
		Provider:      "frankfurter",
		BaseCurrency:  "USD",
		QuoteCurrency: "EUR",
		Rate:          "0.92",
		RateDate:      "2026-02-01",
		IsEstimate:    false,
		FetchedAtUTC:  "2026-02-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if created.ID <= 0 {
		t.Fatalf("expected positive snapshot id, got %d", created.ID)
	}

	fetched, err := repo.GetSnapshotByKey(ctx, "frankfurter", "USD", "EUR", "2026-02-01", false)
	if err != nil {
		t.Fatalf("get snapshot by key: %v", err)
	}

	if fetched.Rate != "0.92" {
		t.Fatalf("expected stored rate 0.92, got %q", fetched.Rate)
	}
	if fetched.IsEstimate {
		t.Fatalf("expected non-estimate snapshot")
	}
}

func TestFXRepoGetSnapshotByKeyNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openFXTestDB(t)
	defer db.Close()

	repo := NewFXRepo(db)
	_, err := repo.GetSnapshotByKey(ctx, "frankfurter", "USD", "EUR", "2026-02-01", false)
	if err == nil {
		t.Fatalf("expected not found error")
	}
	if err != domain.ErrFXRateUnavailable {
		t.Fatalf("expected ErrFXRateUnavailable, got %v", err)
	}
}

func openFXTestDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "fx-test.db")
	db, err := OpenAndMigrate(ctx, dbPath, migrationsDirFromFXRepoTest(t))
	if err != nil {
		t.Fatalf("open and migrate test db: %v", err)
	}
	return db
}

func migrationsDirFromFXRepoTest(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve current file path")
	}

	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	return filepath.Join(projectRoot, "migrations")
}
