package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"boring-budget/internal/domain"
	sqlcqueries "boring-budget/internal/store/sqlite/sqlc"
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
		Note:               "Coffee beans",
	}); err != nil {
		t.Fatalf("add entry 1: %v", err)
	}
	if _, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeIncome,
		AmountMinor:        500,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-15T00:00:00Z",
		CategoryID:         &categoryA,
		Note:               "Salary",
	}); err != nil {
		t.Fatalf("add entry 2: %v", err)
	}
	if _, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        900,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-03-01T00:00:00Z",
		CategoryID:         &categoryB,
		Note:               "RENT",
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

	noteEntries, err := repo.List(ctx, domain.EntryListFilter{NoteContains: "fee"})
	if err != nil {
		t.Fatalf("list by note contains: %v", err)
	}
	if len(noteEntries) != 1 || noteEntries[0].Note != "Coffee beans" {
		t.Fatalf("unexpected note filter entries: %+v", noteEntries)
	}

	upperNoteEntries, err := repo.List(ctx, domain.EntryListFilter{NoteContains: "rent"})
	if err != nil {
		t.Fatalf("list by note contains (case-insensitive): %v", err)
	}
	if len(upperNoteEntries) != 1 || upperNoteEntries[0].Note != "RENT" {
		t.Fatalf("unexpected case-insensitive note filter entries: %+v", upperNoteEntries)
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

func TestEntryRepoListFiltersByPaymentMethodAndCardSelectors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openEntryTestDB(t)
	defer db.Close()

	repo := NewEntryRepo(db)

	debitCardID := insertCardForEntryTest(t, ctx, db, "Debit One", "Daily debit card", "1111", "VISA", "debit", nil)
	creditDueDay := int64(20)
	creditCardID := insertCardForEntryTest(t, ctx, db, "Credit One", "Travel card", "2222", "MASTERCARD", "credit", &creditDueDay)

	if _, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        1000,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("add cash expense: %v", err)
	}

	if _, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        2000,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-02T00:00:00Z",
		PaymentMethod:      domain.PaymentMethodCard,
		PaymentCardID:      &debitCardID,
	}); err != nil {
		t.Fatalf("add debit-card expense: %v", err)
	}

	if _, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        3000,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-03T00:00:00Z",
		PaymentMethod:      domain.PaymentMethodCard,
		PaymentCardID:      &creditCardID,
	}); err != nil {
		t.Fatalf("add credit-card expense: %v", err)
	}

	cashEntries, err := repo.List(ctx, domain.EntryListFilter{PaymentMethod: domain.PaymentMethodCash})
	if err != nil {
		t.Fatalf("list cash entries: %v", err)
	}
	if len(cashEntries) != 1 || cashEntries[0].PaymentMethod != domain.PaymentMethodCash {
		t.Fatalf("unexpected cash entries: %+v", cashEntries)
	}

	cardEntries, err := repo.List(ctx, domain.EntryListFilter{PaymentMethod: domain.PaymentMethodCard})
	if err != nil {
		t.Fatalf("list card entries: %v", err)
	}
	if len(cardEntries) != 2 {
		t.Fatalf("expected 2 card entries, got %d", len(cardEntries))
	}

	creditEntries, err := repo.List(ctx, domain.EntryListFilter{PaymentMethod: domain.PaymentMethodFilterCredit})
	if err != nil {
		t.Fatalf("list credit entries: %v", err)
	}
	if len(creditEntries) != 1 || creditEntries[0].PaymentCardID == nil || *creditEntries[0].PaymentCardID != creditCardID {
		t.Fatalf("unexpected credit entries: %+v", creditEntries)
	}

	debitByID, err := repo.List(ctx, domain.EntryListFilter{PaymentCardID: &debitCardID})
	if err != nil {
		t.Fatalf("list by debit card id: %v", err)
	}
	if len(debitByID) != 1 || debitByID[0].PaymentCardID == nil || *debitByID[0].PaymentCardID != debitCardID {
		t.Fatalf("unexpected debit-card-id entries: %+v", debitByID)
	}

	creditByNickname, err := repo.List(ctx, domain.EntryListFilter{PaymentCardNickname: "credit one"})
	if err != nil {
		t.Fatalf("list by card nickname: %v", err)
	}
	if len(creditByNickname) != 1 || creditByNickname[0].PaymentCardID == nil || *creditByNickname[0].PaymentCardID != creditCardID {
		t.Fatalf("unexpected card-nickname entries: %+v", creditByNickname)
	}

	debitByLookup, err := repo.List(ctx, domain.EntryListFilter{PaymentCardLookup: "daily"})
	if err != nil {
		t.Fatalf("list by card lookup: %v", err)
	}
	if len(debitByLookup) != 1 || debitByLookup[0].PaymentCardID == nil || *debitByLookup[0].PaymentCardID != debitCardID {
		t.Fatalf("unexpected card-lookup entries: %+v", debitByLookup)
	}
}

func TestEntryRepoSyncCreditLiabilityChargeLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openEntryTestDB(t)
	defer db.Close()

	repo := NewEntryRepo(db)

	creditDueDay := int64(15)
	creditCardID := insertCardForEntryTest(t, ctx, db, "Credit One", "Primary credit", "3333", "VISA", "credit", &creditDueDay)
	debitCardID := insertCardForEntryTest(t, ctx, db, "Debit One", "Primary debit", "4444", "VISA", "debit", nil)

	entry, err := repo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        2500,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-10T00:00:00Z",
		PaymentMethod:      domain.PaymentMethodCard,
		PaymentCardID:      &creditCardID,
	})
	if err != nil {
		t.Fatalf("add entry: %v", err)
	}

	if err := repo.SyncCreditLiabilityCharge(ctx, entry.ID); err != nil {
		t.Fatalf("sync credit liability (initial): %v", err)
	}
	assertLiabilityChargeTotal(t, ctx, db, entry.ID, 1, 2500)

	newAmount := int64(1800)
	updated, err := repo.Update(ctx, domain.EntryUpdateInput{
		ID:          entry.ID,
		AmountMinor: &newAmount,
	})
	if err != nil {
		t.Fatalf("update entry amount: %v", err)
	}
	if updated.AmountMinor != 1800 {
		t.Fatalf("unexpected updated amount: %+v", updated)
	}

	if err := repo.SyncCreditLiabilityCharge(ctx, entry.ID); err != nil {
		t.Fatalf("sync credit liability (after amount update): %v", err)
	}
	assertLiabilityChargeTotal(t, ctx, db, entry.ID, 1, 1800)

	if _, err := repo.Update(ctx, domain.EntryUpdateInput{
		ID:             entry.ID,
		SetPaymentCard: true,
		PaymentCardID:  &debitCardID,
	}); err != nil {
		t.Fatalf("update entry payment card to debit: %v", err)
	}
	if err := repo.SyncCreditLiabilityCharge(ctx, entry.ID); err != nil {
		t.Fatalf("sync credit liability (after debit switch): %v", err)
	}
	assertLiabilityChargeTotal(t, ctx, db, entry.ID, 0, 0)

	methodCash := domain.PaymentMethodCash
	if _, err := repo.Update(ctx, domain.EntryUpdateInput{
		ID:               entry.ID,
		SetPaymentMethod: true,
		PaymentMethod:    &methodCash,
	}); err != nil {
		t.Fatalf("update entry payment to cash: %v", err)
	}
	if err := repo.SyncCreditLiabilityCharge(ctx, entry.ID); err != nil {
		t.Fatalf("sync credit liability (after cash switch): %v", err)
	}
	assertLiabilityChargeTotal(t, ctx, db, entry.ID, 0, 0)

	if _, err := repo.Delete(ctx, entry.ID); err != nil {
		t.Fatalf("delete entry: %v", err)
	}
	assertLiabilityChargeTotal(t, ctx, db, entry.ID, 0, 0)
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

func insertCardForEntryTest(t *testing.T, ctx context.Context, db *sql.DB, nickname, description, last4, brand, cardType string, dueDay *int64) int64 {
	t.Helper()

	result, err := db.ExecContext(
		ctx,
		`INSERT INTO cards (nickname, description, last4, brand, card_type, due_day) VALUES (?, ?, ?, ?, ?, ?);`,
		nickname,
		description,
		last4,
		brand,
		cardType,
		nullableInt64(dueDay),
	)
	if err != nil {
		t.Fatalf("insert card: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read inserted card id: %v", err)
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

func assertLiabilityChargeTotal(t *testing.T, ctx context.Context, db *sql.DB, entryID int64, expectedCount int64, expectedTotal int64) {
	t.Helper()

	var count int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM credit_liability_events WHERE reference_transaction_id = ?;`, entryID).Scan(&count); err != nil {
		t.Fatalf("count liability events by reference transaction: %v", err)
	}
	if count != expectedCount {
		t.Fatalf("expected %d liability events, got %d", expectedCount, count)
	}

	var total int64
	if err := db.QueryRowContext(ctx, `SELECT CAST(COALESCE(SUM(amount_minor_signed), 0) AS INTEGER) FROM credit_liability_events WHERE reference_transaction_id = ?;`, entryID).Scan(&total); err != nil {
		t.Fatalf("sum liability events by reference transaction: %v", err)
	}
	if total != expectedTotal {
		t.Fatalf("expected liability sum %d, got %d", expectedTotal, total)
	}
}
