package service

import (
	"context"
	"reflect"
	"testing"

	"boring-budget/internal/domain"
)

type savingsEntryReaderStub struct {
	listFn func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error)
}

func (s *savingsEntryReaderStub) List(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
	return s.listFn(ctx, filter)
}

type savingsEventRepoStub struct {
	addFn  func(ctx context.Context, input domain.SavingsEventAddInput) (domain.SavingsEvent, error)
	listFn func(ctx context.Context, filter domain.SavingsEventListFilter) ([]domain.SavingsEvent, error)
}

func (s *savingsEventRepoStub) AddEvent(ctx context.Context, input domain.SavingsEventAddInput) (domain.SavingsEvent, error) {
	return s.addFn(ctx, input)
}

func (s *savingsEventRepoStub) ListEvents(ctx context.Context, filter domain.SavingsEventListFilter) ([]domain.SavingsEvent, error) {
	return s.listFn(ctx, filter)
}

func TestNewSavingsServiceRequiresDependencies(t *testing.T) {
	t.Parallel()

	_, err := NewSavingsService(nil, &savingsEventRepoStub{})
	if err == nil {
		t.Fatalf("expected error for nil entry reader")
	}

	_, err = NewSavingsService(&savingsEntryReaderStub{}, nil)
	if err == nil {
		t.Fatalf("expected error for nil event repo")
	}
}

func TestSavingsServiceAddTransferNormalizesInput(t *testing.T) {
	t.Parallel()

	var captured domain.SavingsEventAddInput

	svc, err := NewSavingsService(
		&savingsEntryReaderStub{
			listFn: func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
				return nil, nil
			},
		},
		&savingsEventRepoStub{
			addFn: func(ctx context.Context, input domain.SavingsEventAddInput) (domain.SavingsEvent, error) {
				captured = input
				return domain.SavingsEvent{ID: 7, EventType: input.EventType}, nil
			},
			listFn: func(ctx context.Context, filter domain.SavingsEventListFilter) ([]domain.SavingsEvent, error) {
				return nil, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("new savings service: %v", err)
	}

	event, err := svc.AddTransfer(context.Background(), SavingsAddInput{
		AmountMinor:  1250,
		CurrencyCode: "usd",
		EventDateUTC: "2026-01-05",
		Note:         "  reserve fund  ",
	})
	if err != nil {
		t.Fatalf("add transfer: %v", err)
	}
	if event.EventType != domain.SavingsEventTypeTransferToSavings {
		t.Fatalf("unexpected event type: %q", event.EventType)
	}
	if captured.EventType != domain.SavingsEventTypeTransferToSavings {
		t.Fatalf("expected transfer event type, got %q", captured.EventType)
	}
	if captured.CurrencyCode != "USD" {
		t.Fatalf("expected normalized currency USD, got %q", captured.CurrencyCode)
	}
	if captured.EventDateUTC != "2026-01-05T00:00:00Z" {
		t.Fatalf("expected normalized date, got %q", captured.EventDateUTC)
	}
	if captured.Note != "reserve fund" {
		t.Fatalf("expected trimmed note, got %q", captured.Note)
	}
}

func TestSavingsServiceShowAppliesReplayBalanceRules(t *testing.T) {
	t.Parallel()

	svc, err := NewSavingsService(
		&savingsEntryReaderStub{
			listFn: func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
				return []domain.Entry{
					{ID: 1, Type: domain.EntryTypeIncome, AmountMinor: 10000, CurrencyCode: "USD", TransactionDateUTC: "2026-01-01T00:00:00Z"},
					{ID: 2, Type: domain.EntryTypeExpense, AmountMinor: 15000, CurrencyCode: "USD", TransactionDateUTC: "2026-01-03T00:00:00Z"},
				}, nil
			},
		},
		&savingsEventRepoStub{
			addFn: func(ctx context.Context, input domain.SavingsEventAddInput) (domain.SavingsEvent, error) {
				return domain.SavingsEvent{}, nil
			},
			listFn: func(ctx context.Context, filter domain.SavingsEventListFilter) ([]domain.SavingsEvent, error) {
				return []domain.SavingsEvent{
					{ID: 1, EventType: domain.SavingsEventTypeTransferToSavings, AmountMinor: 3000, CurrencyCode: "USD", EventDateUTC: "2026-01-02T00:00:00Z"},
					{ID: 2, EventType: domain.SavingsEventTypeIndependentAdd, AmountMinor: 500, CurrencyCode: "USD", EventDateUTC: "2026-01-04T00:00:00Z"},
				}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("new savings service: %v", err)
	}

	result, err := svc.Show(context.Background(), SavingsShowRequest{
		IncludeLifetime: true,
	})
	if err != nil {
		t.Fatalf("show savings: %v", err)
	}
	if result.Lifetime == nil {
		t.Fatalf("expected lifetime view")
	}

	expected := []domain.SavingsCurrencyBalance{
		{
			CurrencyCode:        "USD",
			GeneralBalanceMinor: -5000,
			SavingsBalanceMinor: 500,
			TotalBalanceMinor:   -4500,
		},
	}
	if !reflect.DeepEqual(result.Lifetime.ByCurrency, expected) {
		t.Fatalf("unexpected lifetime balances: %+v", result.Lifetime.ByCurrency)
	}
}

func TestSavingsServiceShowDefaultsToBothViewsAndNormalizesRange(t *testing.T) {
	t.Parallel()

	var entryFilters []domain.EntryListFilter
	var eventFilters []domain.SavingsEventListFilter

	svc, err := NewSavingsService(
		&savingsEntryReaderStub{
			listFn: func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
				entryFilters = append(entryFilters, filter)
				return nil, nil
			},
		},
		&savingsEventRepoStub{
			addFn: func(ctx context.Context, input domain.SavingsEventAddInput) (domain.SavingsEvent, error) {
				return domain.SavingsEvent{}, nil
			},
			listFn: func(ctx context.Context, filter domain.SavingsEventListFilter) ([]domain.SavingsEvent, error) {
				eventFilters = append(eventFilters, filter)
				return nil, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("new savings service: %v", err)
	}

	result, err := svc.Show(context.Background(), SavingsShowRequest{
		RangeFromUTC: "2026-02-01",
		RangeToUTC:   "2026-02-28",
	})
	if err != nil {
		t.Fatalf("show savings: %v", err)
	}
	if result.Lifetime == nil || result.Range == nil {
		t.Fatalf("expected both views when none explicitly selected")
	}
	if len(entryFilters) != 2 || len(eventFilters) != 2 {
		t.Fatalf("expected two list calls for entries and events")
	}
	if entryFilters[1].DateFromUTC != "2026-02-01T00:00:00Z" {
		t.Fatalf("unexpected normalized range from: %q", entryFilters[1].DateFromUTC)
	}
	if entryFilters[1].DateToUTC != "2026-02-28T23:59:59.999999999Z" {
		t.Fatalf("unexpected normalized range to: %q", entryFilters[1].DateToUTC)
	}
	if eventFilters[1].DateFromUTC != "2026-02-01T00:00:00Z" {
		t.Fatalf("unexpected event range from: %q", eventFilters[1].DateFromUTC)
	}
	if eventFilters[1].DateToUTC != "2026-02-28T23:59:59.999999999Z" {
		t.Fatalf("unexpected event range to: %q", eventFilters[1].DateToUTC)
	}
}

func TestSavingsServiceShowOrdersReplayByDateThenID(t *testing.T) {
	t.Parallel()

	svc, err := NewSavingsService(
		&savingsEntryReaderStub{
			listFn: func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
				return []domain.Entry{
					{ID: 2, Type: domain.EntryTypeIncome, AmountMinor: 1000, CurrencyCode: "USD", TransactionDateUTC: "2026-01-01T00:00:00Z"},
					{ID: 1, Type: domain.EntryTypeExpense, AmountMinor: 500, CurrencyCode: "USD", TransactionDateUTC: "2026-01-01T00:00:00Z"},
				}, nil
			},
		},
		&savingsEventRepoStub{
			addFn: func(ctx context.Context, input domain.SavingsEventAddInput) (domain.SavingsEvent, error) {
				return domain.SavingsEvent{}, nil
			},
			listFn: func(ctx context.Context, filter domain.SavingsEventListFilter) ([]domain.SavingsEvent, error) {
				return []domain.SavingsEvent{
					{ID: 2, EventType: domain.SavingsEventTypeIndependentAdd, AmountMinor: 100, CurrencyCode: "USD", EventDateUTC: "2026-01-01T00:00:00Z"},
					{ID: 1, EventType: domain.SavingsEventTypeTransferToSavings, AmountMinor: 200, CurrencyCode: "USD", EventDateUTC: "2026-01-01T00:00:00Z"},
				}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("new savings service: %v", err)
	}

	result, err := svc.Show(context.Background(), SavingsShowRequest{IncludeLifetime: true})
	if err != nil {
		t.Fatalf("show savings: %v", err)
	}
	if result.Lifetime == nil {
		t.Fatalf("expected lifetime view")
	}

	expected := []domain.SavingsCurrencyBalance{
		{
			CurrencyCode:        "USD",
			GeneralBalanceMinor: 300,
			SavingsBalanceMinor: 300,
			TotalBalanceMinor:   600,
		},
	}
	if !reflect.DeepEqual(result.Lifetime.ByCurrency, expected) {
		t.Fatalf("unexpected replay ordering result: %+v", result.Lifetime.ByCurrency)
	}
}
