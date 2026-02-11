package service

import (
	"context"
	"reflect"
	"testing"

	"budgetto/internal/domain"
)

type balanceEntryReaderStub struct {
	listFn func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error)
}

func (s *balanceEntryReaderStub) List(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
	return s.listFn(ctx, filter)
}

type balanceFXConverterStub struct {
	convertFn func(ctx context.Context, amountMinor int64, fromCurrency, toCurrency, transactionDateUTC string) (domain.ConvertedAmount, error)
}

func (s *balanceFXConverterStub) Convert(ctx context.Context, amountMinor int64, fromCurrency, toCurrency, transactionDateUTC string) (domain.ConvertedAmount, error) {
	return s.convertFn(ctx, amountMinor, fromCurrency, toCurrency, transactionDateUTC)
}

func TestNewBalanceServiceRequiresEntryReader(t *testing.T) {
	t.Parallel()

	_, err := NewBalanceService(nil)
	if err == nil {
		t.Fatalf("expected error for nil entry reader")
	}
}

func TestBalanceServiceComputeLifetimeAndRange(t *testing.T) {
	t.Parallel()

	var filters []domain.EntryListFilter

	svc, err := NewBalanceService(&balanceEntryReaderStub{
		listFn: func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
			filters = append(filters, filter)

			if filter.DateFromUTC == "" && filter.DateToUTC == "" {
				return []domain.Entry{
					{ID: 1, Type: domain.EntryTypeIncome, AmountMinor: 10000, CurrencyCode: "USD", TransactionDateUTC: "2026-01-05T00:00:00Z"},
					{ID: 2, Type: domain.EntryTypeExpense, AmountMinor: 3000, CurrencyCode: "USD", TransactionDateUTC: "2026-01-06T00:00:00Z"},
					{ID: 3, Type: domain.EntryTypeIncome, AmountMinor: 5000, CurrencyCode: "EUR", TransactionDateUTC: "2026-01-07T00:00:00Z"},
				}, nil
			}

			return []domain.Entry{
				{ID: 4, Type: domain.EntryTypeIncome, AmountMinor: 4000, CurrencyCode: "USD", TransactionDateUTC: "2026-02-10T00:00:00Z"},
				{ID: 5, Type: domain.EntryTypeExpense, AmountMinor: 1000, CurrencyCode: "USD", TransactionDateUTC: "2026-02-11T00:00:00Z"},
				{ID: 6, Type: domain.EntryTypeExpense, AmountMinor: 500, CurrencyCode: "EUR", TransactionDateUTC: "2026-02-12T00:00:00Z"},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("new balance service: %v", err)
	}

	result, err := svc.Compute(context.Background(), BalanceRequest{
		IncludeLifetime: true,
		IncludeRange:    true,
		RangeFromUTC:    "2026-02-01",
		RangeToUTC:      "2026-02-28",
		LabelIDs:        []int64{8, 2, 8},
		LabelMode:       "all",
	})
	if err != nil {
		t.Fatalf("compute balance: %v", err)
	}

	if len(filters) != 2 {
		t.Fatalf("expected 2 list calls, got %d", len(filters))
	}

	if !reflect.DeepEqual(filters[0].LabelIDs, []int64{2, 8}) || filters[0].LabelMode != domain.LabelFilterModeAll {
		t.Fatalf("unexpected normalized label filter for lifetime: %+v", filters[0])
	}

	if filters[1].DateFromUTC != "2026-02-01T00:00:00Z" {
		t.Fatalf("unexpected range date_from filter: %q", filters[1].DateFromUTC)
	}
	if filters[1].DateToUTC != "2026-02-28T23:59:59.999999999Z" {
		t.Fatalf("unexpected range date_to filter: %q", filters[1].DateToUTC)
	}

	if result.Lifetime == nil || result.Range == nil {
		t.Fatalf("expected both lifetime and range results, got %+v", result)
	}

	if !reflect.DeepEqual(result.Lifetime.ByCurrency, []domain.CurrencyNet{
		{CurrencyCode: "EUR", NetMinor: 5000},
		{CurrencyCode: "USD", NetMinor: 7000},
	}) {
		t.Fatalf("unexpected lifetime totals: %+v", result.Lifetime.ByCurrency)
	}

	if !reflect.DeepEqual(result.Range.ByCurrency, []domain.CurrencyNet{
		{CurrencyCode: "EUR", NetMinor: -500},
		{CurrencyCode: "USD", NetMinor: 3000},
	}) {
		t.Fatalf("unexpected range totals: %+v", result.Range.ByCurrency)
	}
}

func TestBalanceServiceComputeDefaultsToBothViews(t *testing.T) {
	t.Parallel()

	svc, err := NewBalanceService(&balanceEntryReaderStub{
		listFn: func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("new balance service: %v", err)
	}

	result, err := svc.Compute(context.Background(), BalanceRequest{})
	if err != nil {
		t.Fatalf("compute balance default views: %v", err)
	}

	if result.Lifetime == nil || result.Range == nil {
		t.Fatalf("expected both views when none selected, got %+v", result)
	}
}

func TestBalanceServiceComputeSupportsConvertedViews(t *testing.T) {
	t.Parallel()

	svc, err := NewBalanceService(
		&balanceEntryReaderStub{
			listFn: func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
				return []domain.Entry{
					{ID: 1, Type: domain.EntryTypeIncome, AmountMinor: 1000, CurrencyCode: "USD", TransactionDateUTC: "2026-01-01T00:00:00Z"},
					{ID: 2, Type: domain.EntryTypeExpense, AmountMinor: 400, CurrencyCode: "EUR", TransactionDateUTC: "2026-12-01T00:00:00Z"},
				}, nil
			},
		},
		WithBalanceFXConverter(&balanceFXConverterStub{
			convertFn: func(ctx context.Context, amountMinor int64, fromCurrency, toCurrency, transactionDateUTC string) (domain.ConvertedAmount, error) {
				return domain.ConvertedAmount{
					AmountMinor: amountMinor,
					Snapshot: domain.FXRateSnapshot{
						IsEstimate: transactionDateUTC == "2026-12-01T00:00:00Z",
					},
				}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("new balance service: %v", err)
	}

	result, err := svc.Compute(context.Background(), BalanceRequest{
		IncludeLifetime: true,
		ConvertTo:       "USD",
	})
	if err != nil {
		t.Fatalf("compute converted balance: %v", err)
	}

	if result.LifetimeConverted == nil {
		t.Fatalf("expected lifetime converted view")
	}
	if result.LifetimeConverted.NetMinor != 600 {
		t.Fatalf("expected converted net 600, got %d", result.LifetimeConverted.NetMinor)
	}
	if !result.LifetimeConverted.UsedEstimateRate {
		t.Fatalf("expected converted view to signal estimate usage")
	}
}
