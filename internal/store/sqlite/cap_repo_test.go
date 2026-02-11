package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"boring-budget/internal/domain"
)

func TestCapRepoSetShowAndHistory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openCapTestDB(t)
	defer db.Close()

	repo := NewCapRepo(db)

	current, change, err := repo.Set(ctx, domain.CapSetInput{
		MonthKey:     "2026-02",
		AmountMinor:  50000,
		CurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("set first cap: %v", err)
	}

	if current.MonthKey != "2026-02" || current.AmountMinor != 50000 || current.CurrencyCode != "USD" {
		t.Fatalf("unexpected current cap after first set: %+v", current)
	}
	if change.OldAmountMinor != nil {
		t.Fatalf("expected first change old amount to be nil, got %v", change.OldAmountMinor)
	}
	if change.NewAmountMinor != 50000 {
		t.Fatalf("expected first change new amount 50000, got %d", change.NewAmountMinor)
	}

	updated, updateChange, err := repo.Set(ctx, domain.CapSetInput{
		MonthKey:     "2026-02",
		AmountMinor:  65000,
		CurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("set second cap: %v", err)
	}

	if updated.AmountMinor != 65000 {
		t.Fatalf("expected updated cap amount 65000, got %d", updated.AmountMinor)
	}
	if updateChange.OldAmountMinor == nil || *updateChange.OldAmountMinor != 50000 {
		t.Fatalf("expected old amount 50000 in change, got %v", updateChange.OldAmountMinor)
	}
	if updateChange.NewAmountMinor != 65000 {
		t.Fatalf("expected new amount 65000 in change, got %d", updateChange.NewAmountMinor)
	}

	history, err := repo.ListChangesByMonth(ctx, "2026-02")
	if err != nil {
		t.Fatalf("list cap history: %v", err)
	}

	if len(history) != 2 {
		t.Fatalf("expected 2 cap changes, got %d", len(history))
	}
	if history[0].OldAmountMinor != nil || history[0].NewAmountMinor != 50000 {
		t.Fatalf("unexpected first change: %+v", history[0])
	}
	if history[1].OldAmountMinor == nil || *history[1].OldAmountMinor != 50000 || history[1].NewAmountMinor != 65000 {
		t.Fatalf("unexpected second change: %+v", history[1])
	}
}

func TestCapRepoGetByMonthNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openCapTestDB(t)
	defer db.Close()

	repo := NewCapRepo(db)

	_, err := repo.GetByMonth(ctx, "2026-02")
	if !errors.Is(err, domain.ErrCapNotFound) {
		t.Fatalf("expected ErrCapNotFound, got %v", err)
	}
}

func TestCapRepoGetExpenseTotalByMonthAndCurrency(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openCapTestDB(t)
	defer db.Close()

	repo := NewCapRepo(db)

	insertCapTestTransaction(t, ctx, db, "expense", 1200, "USD", "2026-02-01T00:00:00Z", nil)
	insertCapTestTransaction(t, ctx, db, "expense", 800, "USD", "2026-02-10T12:00:00Z", nil)
	insertCapTestTransaction(t, ctx, db, "income", 3000, "USD", "2026-02-10T12:00:00Z", nil)
	insertCapTestTransaction(t, ctx, db, "expense", 9999, "EUR", "2026-02-10T12:00:00Z", nil)
	insertCapTestTransaction(t, ctx, db, "expense", 4444, "USD", "2026-03-01T00:00:00Z", nil)
	deletedAtUTC := "2026-02-11T00:00:00Z"
	insertCapTestTransaction(t, ctx, db, "expense", 2000, "USD", "2026-02-15T00:00:00Z", &deletedAtUTC)

	total, err := repo.GetExpenseTotalByMonthAndCurrency(ctx, "2026-02", "USD")
	if err != nil {
		t.Fatalf("sum expense total: %v", err)
	}

	if total != 2000 {
		t.Fatalf("expected expense total 2000, got %d", total)
	}
}

func openCapTestDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "cap-test.db")
	db, err := OpenAndMigrate(ctx, dbPath, capMigrationsDirFromThisFile(t))
	if err != nil {
		t.Fatalf("open and migrate test db: %v", err)
	}
	return db
}

func capMigrationsDirFromThisFile(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve current file path")
	}

	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	return filepath.Join(projectRoot, "migrations")
}

func insertCapTestTransaction(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	entryType string,
	amountMinor int64,
	currencyCode string,
	transactionDateUTC string,
	deletedAtUTC *string,
) {
	t.Helper()

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO transactions (
			type,
			amount_minor,
			currency_code,
			transaction_date_utc,
			note,
			deleted_at_utc
		) VALUES (?, ?, ?, ?, ?, ?);`,
		entryType,
		amountMinor,
		currencyCode,
		transactionDateUTC,
		"cap test",
		deletedAtUTC,
	); err != nil {
		t.Fatalf("insert transaction for cap test: %v", err)
	}
}
