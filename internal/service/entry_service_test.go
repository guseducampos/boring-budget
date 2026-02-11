package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"budgetto/internal/domain"
)

type entryRepoStub struct {
	addFn    func(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error)
	listFn   func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error)
	deleteFn func(ctx context.Context, id int64) (domain.EntryDeleteResult, error)
}

func (s *entryRepoStub) Add(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error) {
	return s.addFn(ctx, input)
}

func (s *entryRepoStub) List(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
	return s.listFn(ctx, filter)
}

func (s *entryRepoStub) Delete(ctx context.Context, id int64) (domain.EntryDeleteResult, error) {
	return s.deleteFn(ctx, id)
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
		addFn:  func(context.Context, domain.EntryAddInput) (domain.Entry, error) { return domain.Entry{}, nil },
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
