package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"boring-budget/internal/domain"
)

type entryRepoStub struct {
	addFn    func(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error)
	updateFn func(ctx context.Context, input domain.EntryUpdateInput) (domain.Entry, error)
	listFn   func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error)
	deleteFn func(ctx context.Context, id int64) (domain.EntryDeleteResult, error)
}

func (s *entryRepoStub) Add(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error) {
	return s.addFn(ctx, input)
}

func (s *entryRepoStub) Update(ctx context.Context, input domain.EntryUpdateInput) (domain.Entry, error) {
	return s.updateFn(ctx, input)
}

func (s *entryRepoStub) List(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
	return s.listFn(ctx, filter)
}

func (s *entryRepoStub) Delete(ctx context.Context, id int64) (domain.EntryDeleteResult, error) {
	return s.deleteFn(ctx, id)
}

type entryCapLookupStub struct {
	getByMonthFn   func(ctx context.Context, monthKey string) (domain.MonthlyCap, error)
	expenseTotalFn func(ctx context.Context, monthKey, currencyCode string) (int64, error)
}

func (s *entryCapLookupStub) GetByMonth(ctx context.Context, monthKey string) (domain.MonthlyCap, error) {
	return s.getByMonthFn(ctx, monthKey)
}

func (s *entryCapLookupStub) GetExpenseTotalByMonthAndCurrency(ctx context.Context, monthKey, currencyCode string) (int64, error) {
	return s.expenseTotalFn(ctx, monthKey, currencyCode)
}

func TestNewEntryServiceRequiresRepo(t *testing.T) {
	t.Parallel()

	_, err := NewEntryService(nil)
	if err == nil {
		t.Fatalf("expected error for nil repo")
	}
}

func TestEntryServiceAddNormalizesInput(t *testing.T) {
	t.Parallel()

	categoryID := int64(8)

	svc, err := NewEntryService(&entryRepoStub{
		addFn: func(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error) {
			if input.Type != domain.EntryTypeExpense {
				t.Fatalf("expected type expense, got %q", input.Type)
			}
			if input.CurrencyCode != "USD" {
				t.Fatalf("expected USD currency, got %q", input.CurrencyCode)
			}
			if input.TransactionDateUTC != "2026-02-11T00:00:00Z" {
				t.Fatalf("expected normalized date, got %q", input.TransactionDateUTC)
			}
			expectedLabels := []int64{1, 2}
			if !reflect.DeepEqual(expectedLabels, input.LabelIDs) {
				t.Fatalf("expected normalized labels %v, got %v", expectedLabels, input.LabelIDs)
			}
			if input.Note != "team lunch" {
				t.Fatalf("expected trimmed note, got %q", input.Note)
			}
			if input.CategoryID == nil || *input.CategoryID != categoryID {
				t.Fatalf("expected category id %d, got %v", categoryID, input.CategoryID)
			}
			return domain.Entry{ID: 1, Type: input.Type, LabelIDs: input.LabelIDs}, nil
		},
		updateFn: func(context.Context, domain.EntryUpdateInput) (domain.Entry, error) {
			return domain.Entry{}, nil
		},
		listFn:   func(context.Context, domain.EntryListFilter) ([]domain.Entry, error) { return nil, nil },
		deleteFn: func(context.Context, int64) (domain.EntryDeleteResult, error) { return domain.EntryDeleteResult{}, nil },
	})
	if err != nil {
		t.Fatalf("new entry service: %v", err)
	}

	entry, err := svc.Add(context.Background(), domain.EntryAddInput{
		Type:               "  expense ",
		AmountMinor:        5000,
		CurrencyCode:       " usd ",
		TransactionDateUTC: "2026-02-11",
		CategoryID:         &categoryID,
		LabelIDs:           []int64{2, 1, 2},
		Note:               "  team lunch  ",
	})
	if err != nil {
		t.Fatalf("add entry: %v", err)
	}

	if entry.ID != 1 {
		t.Fatalf("expected entry id 1, got %d", entry.ID)
	}
}

func TestEntryServiceListNormalizesAndAppliesAnyLabelMode(t *testing.T) {
	t.Parallel()

	categoryID := int64(4)
	var capturedFilter domain.EntryListFilter

	svc, err := NewEntryService(&entryRepoStub{
		addFn: func(context.Context, domain.EntryAddInput) (domain.Entry, error) { return domain.Entry{}, nil },
		updateFn: func(context.Context, domain.EntryUpdateInput) (domain.Entry, error) {
			return domain.Entry{}, nil
		},
		listFn: func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
			capturedFilter = filter
			return []domain.Entry{
				{ID: 1, LabelIDs: []int64{1}},
				{ID: 2, LabelIDs: []int64{3}},
				{ID: 3, LabelIDs: []int64{2}},
			}, nil
		},
		deleteFn: func(context.Context, int64) (domain.EntryDeleteResult, error) { return domain.EntryDeleteResult{}, nil },
	})
	if err != nil {
		t.Fatalf("new entry service: %v", err)
	}

	entries, err := svc.List(context.Background(), domain.EntryListFilter{
		Type:        " expense ",
		CategoryID:  &categoryID,
		DateFromUTC: "2026-02-01",
		DateToUTC:   "2026-02-28",
		LabelIDs:    []int64{3, 1, 3},
		LabelMode:   "any",
	})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}

	if capturedFilter.Type != domain.EntryTypeExpense {
		t.Fatalf("expected normalized type expense, got %q", capturedFilter.Type)
	}
	if capturedFilter.DateFromUTC != "2026-02-01T00:00:00Z" {
		t.Fatalf("expected normalized date_from, got %q", capturedFilter.DateFromUTC)
	}
	if capturedFilter.DateToUTC != "2026-02-28T00:00:00Z" {
		t.Fatalf("expected normalized date_to, got %q", capturedFilter.DateToUTC)
	}
	if capturedFilter.LabelMode != domain.LabelFilterModeAny {
		t.Fatalf("expected normalized label mode ANY, got %q", capturedFilter.LabelMode)
	}
	expectedLabelIDs := []int64{1, 3}
	if !reflect.DeepEqual(expectedLabelIDs, capturedFilter.LabelIDs) {
		t.Fatalf("expected normalized label ids %v, got %v", expectedLabelIDs, capturedFilter.LabelIDs)
	}

	if len(entries) != 2 || entries[0].ID != 1 || entries[1].ID != 2 {
		t.Fatalf("unexpected any-filter result: %+v", entries)
	}
}

func TestEntryServiceListAppliesAllAndNoneModes(t *testing.T) {
	t.Parallel()

	entries := []domain.Entry{
		{ID: 1, LabelIDs: []int64{1, 2, 3}},
		{ID: 2, LabelIDs: []int64{1, 3}},
		{ID: 3, LabelIDs: []int64{2}},
	}

	svc, err := NewEntryService(&entryRepoStub{
		addFn: func(context.Context, domain.EntryAddInput) (domain.Entry, error) { return domain.Entry{}, nil },
		updateFn: func(context.Context, domain.EntryUpdateInput) (domain.Entry, error) {
			return domain.Entry{}, nil
		},
		listFn: func(context.Context, domain.EntryListFilter) ([]domain.Entry, error) { return entries, nil },
		deleteFn: func(context.Context, int64) (domain.EntryDeleteResult, error) {
			return domain.EntryDeleteResult{}, nil
		},
	})
	if err != nil {
		t.Fatalf("new entry service: %v", err)
	}

	allResult, err := svc.List(context.Background(), domain.EntryListFilter{
		LabelIDs:  []int64{1, 3},
		LabelMode: domain.LabelFilterModeAll,
	})
	if err != nil {
		t.Fatalf("list entries all mode: %v", err)
	}
	if len(allResult) != 2 || allResult[0].ID != 1 || allResult[1].ID != 2 {
		t.Fatalf("unexpected all-filter result: %+v", allResult)
	}

	noneResult, err := svc.List(context.Background(), domain.EntryListFilter{
		LabelIDs:  []int64{1, 3},
		LabelMode: domain.LabelFilterModeNone,
	})
	if err != nil {
		t.Fatalf("list entries none mode: %v", err)
	}
	if len(noneResult) != 1 || noneResult[0].ID != 3 {
		t.Fatalf("unexpected none-filter result: %+v", noneResult)
	}
}

func TestEntryServiceDeleteRejectsInvalidID(t *testing.T) {
	t.Parallel()

	svc, err := NewEntryService(&entryRepoStub{
		addFn:    func(context.Context, domain.EntryAddInput) (domain.Entry, error) { return domain.Entry{}, nil },
		updateFn: func(context.Context, domain.EntryUpdateInput) (domain.Entry, error) { return domain.Entry{}, nil },
		listFn:   func(context.Context, domain.EntryListFilter) ([]domain.Entry, error) { return nil, nil },
		deleteFn: func(context.Context, int64) (domain.EntryDeleteResult, error) { return domain.EntryDeleteResult{}, nil },
	})
	if err != nil {
		t.Fatalf("new entry service: %v", err)
	}

	_, err = svc.Delete(context.Background(), 0)
	if !errors.Is(err, domain.ErrInvalidEntryID) {
		t.Fatalf("expected ErrInvalidEntryID, got %v", err)
	}
}

func TestEntryServiceAddWithWarningsReturnsCapExceededWarning(t *testing.T) {
	t.Parallel()

	entryDateUTC := "2026-02-11T10:00:00Z"
	svc, err := NewEntryService(
		&entryRepoStub{
			addFn: func(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error) {
				return domain.Entry{
					ID:                 22,
					Type:               input.Type,
					AmountMinor:        input.AmountMinor,
					CurrencyCode:       input.CurrencyCode,
					TransactionDateUTC: input.TransactionDateUTC,
				}, nil
			},
			updateFn: func(context.Context, domain.EntryUpdateInput) (domain.Entry, error) {
				return domain.Entry{}, nil
			},
			listFn:   func(context.Context, domain.EntryListFilter) ([]domain.Entry, error) { return nil, nil },
			deleteFn: func(context.Context, int64) (domain.EntryDeleteResult, error) { return domain.EntryDeleteResult{}, nil },
		},
		WithEntryCapLookup(&entryCapLookupStub{
			getByMonthFn: func(ctx context.Context, monthKey string) (domain.MonthlyCap, error) {
				if monthKey != "2026-02" {
					t.Fatalf("expected month key 2026-02, got %q", monthKey)
				}
				return domain.MonthlyCap{MonthKey: "2026-02", AmountMinor: 50000, CurrencyCode: "USD"}, nil
			},
			expenseTotalFn: func(ctx context.Context, monthKey, currencyCode string) (int64, error) {
				if monthKey != "2026-02" {
					t.Fatalf("expected month key 2026-02 for total, got %q", monthKey)
				}
				if currencyCode != "USD" {
					t.Fatalf("expected USD for total lookup, got %q", currencyCode)
				}
				return 62000, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("new entry service: %v", err)
	}

	addResult, err := svc.AddWithWarnings(context.Background(), domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        12000,
		CurrencyCode:       "USD",
		TransactionDateUTC: entryDateUTC,
	})
	if err != nil {
		t.Fatalf("add with warnings: %v", err)
	}

	if addResult.Entry.ID != 22 {
		t.Fatalf("expected entry id 22, got %d", addResult.Entry.ID)
	}
	if len(addResult.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(addResult.Warnings))
	}

	warning := addResult.Warnings[0]
	if warning.Code != domain.WarningCodeCapExceeded {
		t.Fatalf("expected CAP_EXCEEDED warning, got %q", warning.Code)
	}
	if warning.Message != domain.CapExceededWarningMessage {
		t.Fatalf("unexpected warning message %q", warning.Message)
	}

	details, ok := warning.Details.(domain.CapExceededWarningDetails)
	if !ok {
		t.Fatalf("expected CapExceededWarningDetails, got %T", warning.Details)
	}

	if details.MonthKey != "2026-02" {
		t.Fatalf("expected month key 2026-02, got %q", details.MonthKey)
	}
	if details.CapAmount.AmountMinor != 50000 || details.CapAmount.CurrencyCode != "USD" {
		t.Fatalf("unexpected cap amount details: %+v", details.CapAmount)
	}
	if details.NewSpendTotal.AmountMinor != 62000 || details.NewSpendTotal.CurrencyCode != "USD" {
		t.Fatalf("unexpected new spend details: %+v", details.NewSpendTotal)
	}
	if details.OverspendAmount.AmountMinor != 12000 || details.OverspendAmount.CurrencyCode != "USD" {
		t.Fatalf("unexpected overspend details: %+v", details.OverspendAmount)
	}
}

func TestEntryServiceAddWithWarningsSkipsDifferentCapCurrency(t *testing.T) {
	t.Parallel()

	svc, err := NewEntryService(
		&entryRepoStub{
			addFn: func(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error) {
				return domain.Entry{
					ID:                 23,
					Type:               input.Type,
					AmountMinor:        input.AmountMinor,
					CurrencyCode:       input.CurrencyCode,
					TransactionDateUTC: input.TransactionDateUTC,
				}, nil
			},
			updateFn: func(context.Context, domain.EntryUpdateInput) (domain.Entry, error) {
				return domain.Entry{}, nil
			},
			listFn:   func(context.Context, domain.EntryListFilter) ([]domain.Entry, error) { return nil, nil },
			deleteFn: func(context.Context, int64) (domain.EntryDeleteResult, error) { return domain.EntryDeleteResult{}, nil },
		},
		WithEntryCapLookup(&entryCapLookupStub{
			getByMonthFn: func(ctx context.Context, monthKey string) (domain.MonthlyCap, error) {
				return domain.MonthlyCap{MonthKey: monthKey, AmountMinor: 10000, CurrencyCode: "EUR"}, nil
			},
			expenseTotalFn: func(ctx context.Context, monthKey, currencyCode string) (int64, error) {
				t.Fatalf("did not expect expense total lookup when cap currency differs")
				return 0, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("new entry service: %v", err)
	}

	addResult, err := svc.AddWithWarnings(context.Background(), domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        2000,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-11T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("add with warnings: %v", err)
	}

	if len(addResult.Warnings) != 0 {
		t.Fatalf("expected no warnings for different cap currency, got %+v", addResult.Warnings)
	}
}

func TestEntryServiceUpdateNormalizesInput(t *testing.T) {
	t.Parallel()

	categoryID := int64(3)
	note := "  updated note  "
	entryType := " income "
	currency := " eur "
	dateValue := "2026-02-12"
	amountMinor := int64(900)

	svc, err := NewEntryService(&entryRepoStub{
		addFn: func(context.Context, domain.EntryAddInput) (domain.Entry, error) { return domain.Entry{}, nil },
		updateFn: func(ctx context.Context, input domain.EntryUpdateInput) (domain.Entry, error) {
			if input.ID != 7 {
				t.Fatalf("expected id 7, got %d", input.ID)
			}
			if input.Type == nil || *input.Type != domain.EntryTypeIncome {
				t.Fatalf("expected normalized type income, got %v", input.Type)
			}
			if input.AmountMinor == nil || *input.AmountMinor != 900 {
				t.Fatalf("expected amount 900, got %v", input.AmountMinor)
			}
			if input.CurrencyCode == nil || *input.CurrencyCode != "EUR" {
				t.Fatalf("expected currency EUR, got %v", input.CurrencyCode)
			}
			if input.TransactionDateUTC == nil || *input.TransactionDateUTC != "2026-02-12T00:00:00Z" {
				t.Fatalf("expected normalized date, got %v", input.TransactionDateUTC)
			}
			if !input.SetCategory || input.CategoryID == nil || *input.CategoryID != categoryID {
				t.Fatalf("expected category set to %d, got %+v", categoryID, input)
			}
			if !input.SetLabelIDs || !reflect.DeepEqual(input.LabelIDs, []int64{2, 5}) {
				t.Fatalf("expected normalized label ids [2 5], got %v", input.LabelIDs)
			}
			if !input.SetNote || input.Note == nil || *input.Note != "updated note" {
				t.Fatalf("expected trimmed note, got %+v", input.Note)
			}

			return domain.Entry{
				ID:                 input.ID,
				Type:               *input.Type,
				AmountMinor:        *input.AmountMinor,
				CurrencyCode:       *input.CurrencyCode,
				TransactionDateUTC: *input.TransactionDateUTC,
				CategoryID:         input.CategoryID,
				LabelIDs:           input.LabelIDs,
				Note:               *input.Note,
			}, nil
		},
		listFn:   func(context.Context, domain.EntryListFilter) ([]domain.Entry, error) { return nil, nil },
		deleteFn: func(context.Context, int64) (domain.EntryDeleteResult, error) { return domain.EntryDeleteResult{}, nil },
	})
	if err != nil {
		t.Fatalf("new entry service: %v", err)
	}

	updated, err := svc.Update(context.Background(), domain.EntryUpdateInput{
		ID:                 7,
		Type:               &entryType,
		AmountMinor:        &amountMinor,
		CurrencyCode:       &currency,
		TransactionDateUTC: &dateValue,
		SetCategory:        true,
		CategoryID:         &categoryID,
		SetLabelIDs:        true,
		LabelIDs:           []int64{5, 2, 5},
		SetNote:            true,
		Note:               &note,
	})
	if err != nil {
		t.Fatalf("update entry: %v", err)
	}
	if updated.ID != 7 {
		t.Fatalf("expected updated id 7, got %d", updated.ID)
	}
}

func TestEntryServiceUpdateRejectsNoFields(t *testing.T) {
	t.Parallel()

	svc, err := NewEntryService(&entryRepoStub{
		addFn:    func(context.Context, domain.EntryAddInput) (domain.Entry, error) { return domain.Entry{}, nil },
		updateFn: func(context.Context, domain.EntryUpdateInput) (domain.Entry, error) { return domain.Entry{}, nil },
		listFn:   func(context.Context, domain.EntryListFilter) ([]domain.Entry, error) { return nil, nil },
		deleteFn: func(context.Context, int64) (domain.EntryDeleteResult, error) { return domain.EntryDeleteResult{}, nil },
	})
	if err != nil {
		t.Fatalf("new entry service: %v", err)
	}

	_, err = svc.Update(context.Background(), domain.EntryUpdateInput{ID: 10})
	if !errors.Is(err, domain.ErrNoEntryUpdateFields) {
		t.Fatalf("expected ErrNoEntryUpdateFields, got %v", err)
	}
}

func TestEntryServiceUpdateWithWarningsReturnsCapExceededWarning(t *testing.T) {
	t.Parallel()

	amountMinor := int64(4000)
	svc, err := NewEntryService(
		&entryRepoStub{
			addFn: func(context.Context, domain.EntryAddInput) (domain.Entry, error) { return domain.Entry{}, nil },
			updateFn: func(ctx context.Context, input domain.EntryUpdateInput) (domain.Entry, error) {
				return domain.Entry{
					ID:                 input.ID,
					Type:               domain.EntryTypeExpense,
					AmountMinor:        amountMinor,
					CurrencyCode:       "USD",
					TransactionDateUTC: "2026-02-20T00:00:00Z",
				}, nil
			},
			listFn:   func(context.Context, domain.EntryListFilter) ([]domain.Entry, error) { return nil, nil },
			deleteFn: func(context.Context, int64) (domain.EntryDeleteResult, error) { return domain.EntryDeleteResult{}, nil },
		},
		WithEntryCapLookup(&entryCapLookupStub{
			getByMonthFn: func(ctx context.Context, monthKey string) (domain.MonthlyCap, error) {
				return domain.MonthlyCap{MonthKey: "2026-02", AmountMinor: 3000, CurrencyCode: "USD"}, nil
			},
			expenseTotalFn: func(ctx context.Context, monthKey, currencyCode string) (int64, error) {
				return 4500, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("new entry service: %v", err)
	}

	result, err := svc.UpdateWithWarnings(context.Background(), domain.EntryUpdateInput{
		ID:          22,
		AmountMinor: &amountMinor,
	})
	if err != nil {
		t.Fatalf("update with warnings: %v", err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != domain.WarningCodeCapExceeded {
		t.Fatalf("expected CAP_EXCEEDED warning, got %+v", result.Warnings)
	}
}
