package fx

import (
	"context"
	"fmt"
	"testing"
	"time"

	"boring-budget/internal/domain"
)

type providerStub struct {
	historicalCalls int
	latestCalls     int
	historicalFn    func(base, quote, date string) (RateQuote, error)
	latestFn        func(base, quote string) (RateQuote, error)
}

func (p *providerStub) Name() string { return "stub" }

func (p *providerStub) HistoricalRate(ctx context.Context, baseCurrency, quoteCurrency, date string) (RateQuote, error) {
	p.historicalCalls++
	return p.historicalFn(baseCurrency, quoteCurrency, date)
}

func (p *providerStub) LatestRate(ctx context.Context, baseCurrency, quoteCurrency string) (RateQuote, error) {
	p.latestCalls++
	return p.latestFn(baseCurrency, quoteCurrency)
}

type snapshotStoreStub struct {
	rows map[string]domain.FXRateSnapshot
	next int64
}

func newSnapshotStoreStub() *snapshotStoreStub {
	return &snapshotStoreStub{rows: map[string]domain.FXRateSnapshot{}, next: 1}
}

func snapshotKey(provider, base, quote, date string, isEstimate bool) string {
	return fmt.Sprintf("%s|%s|%s|%s|%t", provider, base, quote, date, isEstimate)
}

func (s *snapshotStoreStub) GetSnapshotByKey(ctx context.Context, provider, baseCurrency, quoteCurrency, rateDate string, isEstimate bool) (domain.FXRateSnapshot, error) {
	row, ok := s.rows[snapshotKey(provider, baseCurrency, quoteCurrency, rateDate, isEstimate)]
	if !ok {
		return domain.FXRateSnapshot{}, domain.ErrFXRateUnavailable
	}
	return row, nil
}

func (s *snapshotStoreStub) CreateSnapshot(ctx context.Context, input domain.FXRateSnapshotCreateInput) (domain.FXRateSnapshot, error) {
	row := domain.FXRateSnapshot{
		ID:            s.next,
		Provider:      input.Provider,
		BaseCurrency:  input.BaseCurrency,
		QuoteCurrency: input.QuoteCurrency,
		Rate:          input.Rate,
		RateDate:      input.RateDate,
		IsEstimate:    input.IsEstimate,
		FetchedAtUTC:  input.FetchedAtUTC,
	}
	s.next++
	s.rows[snapshotKey(row.Provider, row.BaseCurrency, row.QuoteCurrency, row.RateDate, row.IsEstimate)] = row
	return row, nil
}

func TestConverterUsesHistoricalRateAndCachesSnapshot(t *testing.T) {
	t.Parallel()

	provider := &providerStub{
		historicalFn: func(base, quote, date string) (RateQuote, error) {
			if base != "USD" || quote != "EUR" || date != "2026-02-10" {
				t.Fatalf("unexpected historical request: %s %s %s", base, quote, date)
			}
			return RateQuote{
				Provider:      "stub",
				BaseCurrency:  base,
				QuoteCurrency: quote,
				Rate:          "1.5",
				RateDate:      date,
			}, nil
		},
		latestFn: func(base, quote string) (RateQuote, error) {
			t.Fatalf("latest should not be called")
			return RateQuote{}, nil
		},
	}
	store := newSnapshotStoreStub()

	converter, err := NewConverter(provider, store)
	if err != nil {
		t.Fatalf("new converter: %v", err)
	}
	converter.nowFn = func() time.Time {
		return time.Date(2026, time.February, 11, 0, 0, 0, 0, time.UTC)
	}

	result, err := converter.Convert(context.Background(), 100, "USD", "EUR", "2026-02-10")
	if err != nil {
		t.Fatalf("convert historical: %v", err)
	}

	if result.AmountMinor != 150 {
		t.Fatalf("expected converted amount 150, got %d", result.AmountMinor)
	}
	if result.Snapshot.IsEstimate {
		t.Fatalf("expected non-estimate snapshot")
	}
	if provider.historicalCalls != 1 {
		t.Fatalf("expected 1 historical call, got %d", provider.historicalCalls)
	}

	result2, err := converter.Convert(context.Background(), 100, "USD", "EUR", "2026-02-10")
	if err != nil {
		t.Fatalf("convert historical cached: %v", err)
	}
	if result2.AmountMinor != 150 {
		t.Fatalf("expected cached converted amount 150, got %d", result2.AmountMinor)
	}
	if provider.historicalCalls != 1 {
		t.Fatalf("expected cached lookup without provider call, got %d historical calls", provider.historicalCalls)
	}
}

func TestConverterUsesLatestForFutureDatedTransactions(t *testing.T) {
	t.Parallel()

	provider := &providerStub{
		historicalFn: func(base, quote, date string) (RateQuote, error) {
			t.Fatalf("historical should not be called")
			return RateQuote{}, nil
		},
		latestFn: func(base, quote string) (RateQuote, error) {
			if base != "USD" || quote != "EUR" {
				t.Fatalf("unexpected latest request: %s %s", base, quote)
			}
			return RateQuote{
				Provider:      "stub",
				BaseCurrency:  base,
				QuoteCurrency: quote,
				Rate:          "2.0",
				RateDate:      "2026-02-01",
			}, nil
		},
	}
	store := newSnapshotStoreStub()

	converter, err := NewConverter(provider, store)
	if err != nil {
		t.Fatalf("new converter: %v", err)
	}
	converter.nowFn = func() time.Time {
		return time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	}

	result, err := converter.Convert(context.Background(), 100, "USD", "EUR", "2026-03-01")
	if err != nil {
		t.Fatalf("convert future: %v", err)
	}

	if result.AmountMinor != 200 {
		t.Fatalf("expected converted amount 200, got %d", result.AmountMinor)
	}
	if !result.Snapshot.IsEstimate {
		t.Fatalf("expected estimate snapshot for future-dated conversion")
	}
	if provider.latestCalls != 1 {
		t.Fatalf("expected 1 latest call, got %d", provider.latestCalls)
	}
}
