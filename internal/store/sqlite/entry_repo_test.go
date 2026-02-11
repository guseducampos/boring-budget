package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"budgetto/internal/domain"
	sqlcqueries "budgetto/internal/store/sqlite/sqlc"
)

func TestEntryRepoAddListDeleteWithCategoryAndLabels(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openEntryTestDB(t)
	defer db.Close()

	repo := NewEntryRepo(db)

	categoryID := insertCategoryForEntryTest(t, ctx, db, "Food")
	labelA := insertLabelForEntryTest(t, ctx, db, "Work")
	labelB := insertLabelForEntryTest(t, ctx, db, "Lunch")

	entry, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        2500,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-11T12:30:00Z",
		CategoryID:         &categoryID,
		LabelIDs:           []int64{labelB, labelA},
		Note:               "team lunch",
	})
	if err != nil {
		t.Fatalf("add entry: %v", err)
	}
	if entry.ID <= 0 {
		t.Fatalf("expected entry id > 0, got %d", entry.ID)
	}
	if entry.CategoryID == nil || *entry.CategoryID != categoryID {
		t.Fatalf("expected category id %d, got %v", categoryID, entry.CategoryID)
	}

	expectedLabelIDs := []int64{labelA, labelB}
	if !reflect.DeepEqual(expectedLabelIDs, entry.LabelIDs) {
		t.Fatalf("expected sorted label ids %v, got %v", expectedLabelIDs, entry.LabelIDs)
	}

	allEntries, err := repo.List(ctx, domain.EntryListFilter{})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(allEntries) != 1 || allEntries[0].ID != entry.ID {
		t.Fatalf("unexpected list entries: %+v", allEntries)
	}

	deleteResult, err := repo.Delete(ctx, entry.ID)
	if err != nil {
		t.Fatalf("delete entry: %v", err)
	}
	if deleteResult.EntryID != entry.ID {
		t.Fatalf("expected deleted entry id %d, got %d", entry.ID, deleteResult.EntryID)
	}
	if deleteResult.DetachedLabels != 2 {
		t.Fatalf("expected 2 detached labels, got %d", deleteResult.DetachedLabels)
	}
	if deleteResult.DeletedAtUTC == "" {
		t.Fatalf("expected deleted_at_utc to be set")
	}

	assertTransactionSoftDeleted(t, ctx, db, entry.ID)
	assertTransactionLabelLinksSoftDeleted(t, ctx, db, entry.ID, 2)
	assertCategoryStillActive(t, ctx, db, categoryID)
	assertLabelStillActive(t, ctx, db, labelA)
	assertLabelStillActive(t, ctx, db, labelB)

	allEntries, err = repo.List(ctx, domain.EntryListFilter{})
	if err != nil {
		t.Fatalf("list entries after delete: %v", err)
	}
	if len(allEntries) != 0 {
		t.Fatalf("expected no active entries after delete, got %d", len(allEntries))
	}
}

func TestEntryRepoAddRejectsMissingCategoryAndLabel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openEntryTestDB(t)
	defer db.Close()

	repo := NewEntryRepo(db)

	missingCategoryID := int64(999)
	_, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        100,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-11T00:00:00Z",
		CategoryID:         &missingCategoryID,
	})
	if !errors.Is(err, domain.ErrCategoryNotFound) {
		t.Fatalf("expected ErrCategoryNotFound, got %v", err)
	}
	assertTransactionCount(t, ctx, db, 0)

	_, err = repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        100,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-11T00:00:00Z",
		LabelIDs:           []int64{888},
	})
	if !errors.Is(err, domain.ErrLabelNotFound) {
		t.Fatalf("expected ErrLabelNotFound, got %v", err)
	}
	assertTransactionCount(t, ctx, db, 0)
}

func TestEntryRepoListFiltersByTypeCategoryAndDate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openEntryTestDB(t)
	defer db.Close()

	repo := NewEntryRepo(db)

	categoryA := insertCategoryForEntryTest(t, ctx, db, "Category A")
	categoryB := insertCategoryForEntryTest(t, ctx, db, "Category B")

	if _, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        200,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-01T00:00:00Z",
		CategoryID:         &categoryA,
	}); err != nil {
		t.Fatalf("add entry 1: %v", err)
	}
	if _, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeIncome,
		AmountMinor:        500,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-15T00:00:00Z",
		CategoryID:         &categoryA,
	}); err != nil {
		t.Fatalf("add entry 2: %v", err)
	}
	if _, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        900,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-03-01T00:00:00Z",
		CategoryID:         &categoryB,
	}); err != nil {
		t.Fatalf("add entry 3: %v", err)
	}

	expenses, err := repo.List(ctx, domain.EntryListFilter{Type: domain.EntryTypeExpense})
	if err != nil {
		t.Fatalf("list expenses: %v", err)
	}
	if len(expenses) != 2 {
		t.Fatalf("expected 2 expenses, got %d", len(expenses))
	}

	categoryEntries, err := repo.List(ctx, domain.EntryListFilter{CategoryID: &categoryA})
	if err != nil {
		t.Fatalf("list by category: %v", err)
	}
	if len(categoryEntries) != 2 {
		t.Fatalf("expected 2 entries in category A, got %d", len(categoryEntries))
	}

	dateEntries, err := repo.List(ctx, domain.EntryListFilter{
		DateFromUTC: "2026-02-10T00:00:00Z",
		DateToUTC:   "2026-02-28T23:59:59Z",
	})
	if err != nil {
		t.Fatalf("list by date range: %v", err)
	}
	if len(dateEntries) != 1 || dateEntries[0].Type != domain.EntryTypeIncome {
		t.Fatalf("unexpected date range entries: %+v", dateEntries)
	}
}

func TestEntryRepoListReturnsDeterministicLabelsWithoutNPlusOneBehaviorChanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openEntryTestDB(t)
	defer db.Close()

	repo := NewEntryRepo(db)

	categoryID := insertCategoryForEntryTest(t, ctx, db, "Category")
	labelA := insertLabelForEntryTest(t, ctx, db, "A")
	labelB := insertLabelForEntryTest(t, ctx, db, "B")
	labelC := insertLabelForEntryTest(t, ctx, db, "C")

	first, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        100,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-01T00:00:00Z",
		CategoryID:         &categoryID,
		LabelIDs:           []int64{labelC, labelA},
	})
	if err != nil {
		t.Fatalf("add first entry: %v", err)
	}

	second, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        200,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-01T01:00:00Z",
		CategoryID:         &categoryID,
	})
	if err != nil {
		t.Fatalf("add second entry: %v", err)
	}

	third, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        300,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-01T02:00:00Z",
		CategoryID:         &categoryID,
		LabelIDs:           []int64{labelB, labelA},
	})
	if err != nil {
		t.Fatalf("add third entry: %v", err)
	}

	entries, err := repo.List(ctx, domain.EntryListFilter{})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	if entries[0].ID != first.ID || entries[1].ID != second.ID || entries[2].ID != third.ID {
		t.Fatalf("unexpected entry order: %+v", entries)
	}
	if !reflect.DeepEqual(entries[0].LabelIDs, []int64{labelA, labelC}) {
		t.Fatalf("unexpected first entry labels: %v", entries[0].LabelIDs)
	}
	if entries[1].LabelIDs == nil || len(entries[1].LabelIDs) != 0 {
		t.Fatalf("expected second entry labels to be empty slice, got %v", entries[1].LabelIDs)
	}
	if !reflect.DeepEqual(entries[2].LabelIDs, []int64{labelA, labelB}) {
		t.Fatalf("unexpected third entry labels: %v", entries[2].LabelIDs)
	}
}

func TestListActiveEntryLabelIDsForListFilterQueryOrderingAndFiltering(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openEntryTestDB(t)
	defer db.Close()

	repo := NewEntryRepo(db)
	sqlc := sqlcqueries.New(db)

	categoryA := insertCategoryForEntryTest(t, ctx, db, "Category A")
	categoryB := insertCategoryForEntryTest(t, ctx, db, "Category B")
	labelA := insertLabelForEntryTest(t, ctx, db, "A")
	labelB := insertLabelForEntryTest(t, ctx, db, "B")
	labelC := insertLabelForEntryTest(t, ctx, db, "C")

	first, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        1000,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-10T00:00:00Z",
		CategoryID:         &categoryA,
		LabelIDs:           []int64{labelC, labelA},
	})
	if err != nil {
		t.Fatalf("add first entry: %v", err)
	}

	second, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeIncome,
		AmountMinor:        2000,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-11T00:00:00Z",
		CategoryID:         &categoryA,
		LabelIDs:           []int64{labelB},
	})
	if err != nil {
		t.Fatalf("add second entry: %v", err)
	}

	third, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        3000,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-12T00:00:00Z",
		CategoryID:         &categoryB,
		LabelIDs:           []int64{labelB, labelA},
	})
	if err != nil {
		t.Fatalf("add third entry: %v", err)
	}

	allRows, err := sqlc.ListActiveEntryLabelIDsForListFilter(ctx, sqlcqueries.ListActiveEntryLabelIDsForListFilterParams{})
	if err != nil {
		t.Fatalf("list label rows without filter: %v", err)
	}
	expectedAll := []sqlcqueries.ListActiveEntryLabelIDsForListFilterRow{
		{TransactionID: first.ID, LabelID: labelA},
		{TransactionID: first.ID, LabelID: labelC},
		{TransactionID: second.ID, LabelID: labelB},
		{TransactionID: third.ID, LabelID: labelA},
		{TransactionID: third.ID, LabelID: labelB},
	}
	if !reflect.DeepEqual(allRows, expectedAll) {
		t.Fatalf("unexpected unfiltered label rows: got %+v want %+v", allRows, expectedAll)
	}

	filteredRows, err := sqlc.ListActiveEntryLabelIDsForListFilter(ctx, sqlcqueries.ListActiveEntryLabelIDsForListFilterParams{
		EntryType:   domain.EntryTypeExpense,
		CategoryID:  categoryA,
		DateFromUtc: "2026-02-09T00:00:00Z",
		DateToUtc:   "2026-02-10T23:59:59Z",
	})
	if err != nil {
		t.Fatalf("list label rows with filter: %v", err)
	}

	expectedFiltered := []sqlcqueries.ListActiveEntryLabelIDsForListFilterRow{
		{TransactionID: first.ID, LabelID: labelA},
		{TransactionID: first.ID, LabelID: labelC},
	}
	if !reflect.DeepEqual(filteredRows, expectedFiltered) {
		t.Fatalf("unexpected filtered label rows: got %+v want %+v", filteredRows, expectedFiltered)
	}
}

func TestEntryRepoDeleteNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openEntryTestDB(t)
	defer db.Close()

	repo := NewEntryRepo(db)
	_, err := repo.Delete(ctx, 999)
	if !errors.Is(err, domain.ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound, got %v", err)
	}
}

func TestEntryRepoUpdateWithCategoryLabelsAndNote(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openEntryTestDB(t)
	defer db.Close()

	repo := NewEntryRepo(db)

	categoryA := insertCategoryForEntryTest(t, ctx, db, "Category A")
	categoryB := insertCategoryForEntryTest(t, ctx, db, "Category B")
	labelA := insertLabelForEntryTest(t, ctx, db, "A")
	labelB := insertLabelForEntryTest(t, ctx, db, "B")

	entry, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        1000,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-01T00:00:00Z",
		CategoryID:         &categoryA,
		LabelIDs:           []int64{labelA},
		Note:               "old note",
	})
	if err != nil {
		t.Fatalf("add entry: %v", err)
	}

	newType := domain.EntryTypeIncome
	newAmount := int64(3200)
	newCurrency := "EUR"
	newDate := "2026-02-10T00:00:00Z"
	newNote := "updated note"
	updated, err := repo.Update(ctx, domain.EntryUpdateInput{
		ID:                 entry.ID,
		Type:               &newType,
		AmountMinor:        &newAmount,
		CurrencyCode:       &newCurrency,
		TransactionDateUTC: &newDate,
		SetCategory:        true,
		CategoryID:         &categoryB,
		SetLabelIDs:        true,
		LabelIDs:           []int64{labelB},
		SetNote:            true,
		Note:               &newNote,
	})
	if err != nil {
		t.Fatalf("update entry: %v", err)
	}

	if updated.Type != domain.EntryTypeIncome {
		t.Fatalf("expected income type after update, got %q", updated.Type)
	}
	if updated.AmountMinor != 3200 || updated.CurrencyCode != "EUR" || updated.TransactionDateUTC != "2026-02-10T00:00:00Z" {
		t.Fatalf("unexpected core fields after update: %+v", updated)
	}
	if updated.CategoryID == nil || *updated.CategoryID != categoryB {
		t.Fatalf("expected category %d after update, got %+v", categoryB, updated.CategoryID)
	}
	if !reflect.DeepEqual(updated.LabelIDs, []int64{labelB}) {
		t.Fatalf("expected labels [%d], got %v", labelB, updated.LabelIDs)
	}
	if updated.Note != "updated note" {
		t.Fatalf("expected updated note, got %q", updated.Note)
	}
}

func TestEntryRepoUpdateClearCategoryLabelsAndNote(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openEntryTestDB(t)
	defer db.Close()

	repo := NewEntryRepo(db)

	categoryID := insertCategoryForEntryTest(t, ctx, db, "Category")
	labelA := insertLabelForEntryTest(t, ctx, db, "A")
	labelB := insertLabelForEntryTest(t, ctx, db, "B")

	entry, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        1000,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-01T00:00:00Z",
		CategoryID:         &categoryID,
		LabelIDs:           []int64{labelA, labelB},
		Note:               "old note",
	})
	if err != nil {
		t.Fatalf("add entry: %v", err)
	}

	updated, err := repo.Update(ctx, domain.EntryUpdateInput{
		ID:          entry.ID,
		SetCategory: true,
		CategoryID:  nil,
		SetLabelIDs: true,
		LabelIDs:    []int64{},
		SetNote:     true,
		Note:        nil,
	})
	if err != nil {
		t.Fatalf("update clear fields: %v", err)
	}

	if updated.CategoryID != nil {
		t.Fatalf("expected orphaned category, got %v", updated.CategoryID)
	}
	if len(updated.LabelIDs) != 0 {
		t.Fatalf("expected no labels after clear, got %v", updated.LabelIDs)
	}
	if updated.Note != "" {
		t.Fatalf("expected empty note after clear, got %q", updated.Note)
	}
}

func openEntryTestDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "entry-test.db")
	db, err := OpenAndMigrate(ctx, dbPath, entryMigrationsDirFromThisFile(t))
	if err != nil {
		t.Fatalf("open and migrate test db: %v", err)
	}
	return db
}

func entryMigrationsDirFromThisFile(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve current file path")
	}

	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	return filepath.Join(projectRoot, "migrations")
}

func insertCategoryForEntryTest(t *testing.T, ctx context.Context, db *sql.DB, name string) int64 {
	t.Helper()

	result, err := db.ExecContext(ctx, `INSERT INTO categories (name) VALUES (?);`, name)
	if err != nil {
		t.Fatalf("insert category: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read inserted category id: %v", err)
	}

	return id
}

func insertLabelForEntryTest(t *testing.T, ctx context.Context, db *sql.DB, name string) int64 {
	t.Helper()

	result, err := db.ExecContext(ctx, `INSERT INTO labels (name) VALUES (?);`, name)
	if err != nil {
		t.Fatalf("insert label: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read inserted label id: %v", err)
	}

	return id
}

func assertTransactionCount(t *testing.T, ctx context.Context, db *sql.DB, expected int64) {
	t.Helper()

	var count int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions;`).Scan(&count); err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if count != expected {
		t.Fatalf("expected %d transactions, got %d", expected, count)
	}
}

func assertTransactionSoftDeleted(t *testing.T, ctx context.Context, db *sql.DB, entryID int64) {
	t.Helper()

	var deletedAt sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT deleted_at_utc FROM transactions WHERE id = ?;`, entryID).Scan(&deletedAt); err != nil {
		t.Fatalf("query transaction deleted_at_utc: %v", err)
	}
	if !deletedAt.Valid || deletedAt.String == "" {
		t.Fatalf("expected transaction to be soft deleted")
	}
}

func assertTransactionLabelLinksSoftDeleted(t *testing.T, ctx context.Context, db *sql.DB, entryID int64, expected int64) {
	t.Helper()

	var count int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transaction_labels WHERE transaction_id = ? AND deleted_at_utc IS NOT NULL;`, entryID).Scan(&count); err != nil {
		t.Fatalf("count soft deleted transaction labels: %v", err)
	}
	if count != expected {
		t.Fatalf("expected %d soft deleted transaction labels, got %d", expected, count)
	}
}

func assertCategoryStillActive(t *testing.T, ctx context.Context, db *sql.DB, categoryID int64) {
	t.Helper()

	var deletedAt sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT deleted_at_utc FROM categories WHERE id = ?;`, categoryID).Scan(&deletedAt); err != nil {
		t.Fatalf("query category deleted_at_utc: %v", err)
	}
	if deletedAt.Valid {
		t.Fatalf("expected category to remain active")
	}
}

func assertLabelStillActive(t *testing.T, ctx context.Context, db *sql.DB, labelID int64) {
	t.Helper()

	var deletedAt sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT deleted_at_utc FROM labels WHERE id = ?;`, labelID).Scan(&deletedAt); err != nil {
		t.Fatalf("query label deleted_at_utc: %v", err)
	}
	if deletedAt.Valid {
		t.Fatalf("expected label to remain active")
	}
}
