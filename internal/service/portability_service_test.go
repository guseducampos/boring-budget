package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"budgetto/internal/domain"
	sqlitestore "budgetto/internal/store/sqlite"
)

func TestPortabilityServiceImportRollsBackBatchWhenOneRecordFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	portabilitySvc, _, db := newPortabilityServiceTestHarness(t)
	defer db.Close()

	importPath := writePortabilityImportJSON(t, []portabilityEntryRecord{
		{
			Type:               domain.EntryTypeIncome,
			AmountMinor:        10000,
			CurrencyCode:       "USD",
			TransactionDateUTC: "2026-02-01T00:00:00Z",
			Note:               "salary",
		},
		{
			Type:               domain.EntryTypeExpense,
			AmountMinor:        1200,
			CurrencyCode:       "USD",
			TransactionDateUTC: "2026-02-02T00:00:00Z",
			LabelIDs:           []int64{999999},
			Note:               "missing label",
		},
	})

	_, err := portabilitySvc.Import(ctx, PortabilityFormatJSON, importPath, false)
	if !errors.Is(err, domain.ErrLabelNotFound) {
		t.Fatalf("expected ErrLabelNotFound, got %v", err)
	}

	if count := activePortabilityTransactionCount(t, ctx, db); count != 0 {
		t.Fatalf("expected rollback to keep zero transactions, got %d", count)
	}
}

func TestPortabilityServiceImportIdempotentKeepsDuplicateHandling(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	portabilitySvc, entrySvc, db := newPortabilityServiceTestHarness(t)
	defer db.Close()

	if _, err := entrySvc.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeIncome,
		AmountMinor:        9000,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-03-01T10:00:00Z",
		Note:               "existing salary",
	}); err != nil {
		t.Fatalf("seed existing entry: %v", err)
	}

	importPath := writePortabilityImportJSON(t, []portabilityEntryRecord{
		{
			Type:               domain.EntryTypeIncome,
			AmountMinor:        9000,
			CurrencyCode:       "USD",
			TransactionDateUTC: "2026-03-01T10:00:00Z",
			Note:               "existing salary",
		},
		{
			Type:               domain.EntryTypeExpense,
			AmountMinor:        2500,
			CurrencyCode:       "USD",
			TransactionDateUTC: "2026-03-02T00:00:00Z",
			Note:               "new groceries",
		},
		{
			Type:               domain.EntryTypeExpense,
			AmountMinor:        2500,
			CurrencyCode:       "USD",
			TransactionDateUTC: "2026-03-02T00:00:00Z",
			Note:               "new groceries",
		},
	})

	firstImport, err := portabilitySvc.Import(ctx, PortabilityFormatJSON, importPath, true)
	if err != nil {
		t.Fatalf("first idempotent import: %v", err)
	}
	if firstImport.Imported != 1 {
		t.Fatalf("expected imported=1 on first run, got %d", firstImport.Imported)
	}
	if firstImport.Skipped != 2 {
		t.Fatalf("expected skipped=2 on first run, got %d", firstImport.Skipped)
	}
	if len(firstImport.Warnings) != 0 {
		t.Fatalf("expected no warnings on first run, got %+v", firstImport.Warnings)
	}

	secondImport, err := portabilitySvc.Import(ctx, PortabilityFormatJSON, importPath, true)
	if err != nil {
		t.Fatalf("second idempotent import: %v", err)
	}
	if secondImport.Imported != 0 {
		t.Fatalf("expected imported=0 on second run, got %d", secondImport.Imported)
	}
	if secondImport.Skipped != 3 {
		t.Fatalf("expected skipped=3 on second run, got %d", secondImport.Skipped)
	}
	if len(secondImport.Warnings) != 0 {
		t.Fatalf("expected no warnings on second run, got %+v", secondImport.Warnings)
	}

	if count := activePortabilityTransactionCount(t, ctx, db); count != 2 {
		t.Fatalf("expected exactly two active transactions after idempotent imports, got %d", count)
	}
}

func newPortabilityServiceTestHarness(t *testing.T) (*PortabilityService, *EntryService, *sql.DB) {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "portability-service-test.db")
	db, err := sqlitestore.OpenAndMigrate(ctx, dbPath, portabilityMigrationsDirFromThisFile(t))
	if err != nil {
		t.Fatalf("open and migrate portability test db: %v", err)
	}

	entryRepo := sqlitestore.NewEntryRepo(db)
	capRepo := sqlitestore.NewCapRepo(db)
	entrySvc, err := NewEntryService(entryRepo, WithEntryCapLookup(capRepo))
	if err != nil {
		t.Fatalf("new entry service: %v", err)
	}

	portabilitySvc, err := NewPortabilityService(entrySvc, db)
	if err != nil {
		t.Fatalf("new portability service: %v", err)
	}

	return portabilitySvc, entrySvc, db
}

func portabilityMigrationsDirFromThisFile(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve current file path")
	}

	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	return filepath.Join(projectRoot, "migrations")
}

func writePortabilityImportJSON(t *testing.T, records []portabilityEntryRecord) string {
	t.Helper()

	filePath := filepath.Join(t.TempDir(), "import.json")
	payload := portabilityJSONEnvelope{Entries: records}
	content, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal import payload: %v", err)
	}

	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatalf("write import payload: %v", err)
	}

	return filePath
}

func activePortabilityTransactionCount(t *testing.T, ctx context.Context, db *sql.DB) int64 {
	t.Helper()

	var count int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE deleted_at_utc IS NULL;`).Scan(&count); err != nil {
		t.Fatalf("count active transactions: %v", err)
	}

	return count
}
