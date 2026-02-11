package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"boring-budget/internal/domain"
)

type capRepoStub struct {
	setFn                     func(ctx context.Context, input domain.CapSetInput) (domain.MonthlyCap, domain.MonthlyCapChange, error)
	getByMonthFn              func(ctx context.Context, monthKey string) (domain.MonthlyCap, error)
	listChangesByMonthFn      func(ctx context.Context, monthKey string) ([]domain.MonthlyCapChange, error)
	expenseTotalByMonthCurFn  func(ctx context.Context, monthKey, currencyCode string) (int64, error)
}

func (s *capRepoStub) Set(ctx context.Context, input domain.CapSetInput) (domain.MonthlyCap, domain.MonthlyCapChange, error) {
	return s.setFn(ctx, input)
}

func (s *capRepoStub) GetByMonth(ctx context.Context, monthKey string) (domain.MonthlyCap, error) {
	return s.getByMonthFn(ctx, monthKey)
}

func (s *capRepoStub) ListChangesByMonth(ctx context.Context, monthKey string) ([]domain.MonthlyCapChange, error) {
	return s.listChangesByMonthFn(ctx, monthKey)
}

func (s *capRepoStub) GetExpenseTotalByMonthAndCurrency(ctx context.Context, monthKey, currencyCode string) (int64, error) {
	return s.expenseTotalByMonthCurFn(ctx, monthKey, currencyCode)
}

func TestNewCapServiceRequiresRepo(t *testing.T) {
	t.Parallel()

	_, err := NewCapService(nil)
	if err == nil {
		t.Fatalf("expected error for nil repo")
	}
}

func TestCapServiceSetNormalizesInput(t *testing.T) {
	t.Parallel()

	var received domain.CapSetInput
	svc, err := NewCapService(&capRepoStub{
		setFn: func(ctx context.Context, input domain.CapSetInput) (domain.MonthlyCap, domain.MonthlyCapChange, error) {
			received = input
			return domain.MonthlyCap{MonthKey: input.MonthKey, AmountMinor: input.AmountMinor, CurrencyCode: input.CurrencyCode}, domain.MonthlyCapChange{MonthKey: input.MonthKey}, nil
		},
		getByMonthFn:             func(context.Context, string) (domain.MonthlyCap, error) { return domain.MonthlyCap{}, nil },
		listChangesByMonthFn:     func(context.Context, string) ([]domain.MonthlyCapChange, error) { return nil, nil },
		expenseTotalByMonthCurFn: func(context.Context, string, string) (int64, error) { return 0, nil },
	})
	if err != nil {
		t.Fatalf("new cap service: %v", err)
	}

	_, _, err = svc.Set(context.Background(), domain.CapSetInput{
		MonthKey:     "2026-02",
		AmountMinor:  50000,
		CurrencyCode: " usd ",
	})
	if err != nil {
		t.Fatalf("set cap: %v", err)
	}

	if received.MonthKey != "2026-02" {
		t.Fatalf("expected normalized month key 2026-02, got %q", received.MonthKey)
	}
	if received.CurrencyCode != "USD" {
		t.Fatalf("expected normalized currency USD, got %q", received.CurrencyCode)
	}
}

func TestCapServiceShowAndHistoryNormalizeMonth(t *testing.T) {
	t.Parallel()

	var showMonthKey string
	var historyMonthKey string
	expectedHistory := []domain.MonthlyCapChange{
		{ID: 1, MonthKey: "2026-02", NewAmountMinor: 50000, CurrencyCode: "USD"},
		{ID: 2, MonthKey: "2026-02", NewAmountMinor: 65000, CurrencyCode: "USD"},
	}

	svc, err := NewCapService(&capRepoStub{
		setFn: func(context.Context, domain.CapSetInput) (domain.MonthlyCap, domain.MonthlyCapChange, error) {
			return domain.MonthlyCap{}, domain.MonthlyCapChange{}, nil
		},
		getByMonthFn: func(ctx context.Context, monthKey string) (domain.MonthlyCap, error) {
			showMonthKey = monthKey
			return domain.MonthlyCap{MonthKey: monthKey, AmountMinor: 65000, CurrencyCode: "USD"}, nil
		},
		listChangesByMonthFn: func(ctx context.Context, monthKey string) ([]domain.MonthlyCapChange, error) {
			historyMonthKey = monthKey
			return expectedHistory, nil
		},
		expenseTotalByMonthCurFn: func(context.Context, string, string) (int64, error) {
			return 0, nil
		},
	})
	if err != nil {
		t.Fatalf("new cap service: %v", err)
	}

	_, err = svc.Show(context.Background(), " 2026-02 ")
	if err != nil {
		t.Fatalf("show cap: %v", err)
	}
	if showMonthKey != "2026-02" {
		t.Fatalf("expected normalized show month key 2026-02, got %q", showMonthKey)
	}

	history, err := svc.History(context.Background(), "2026-02")
	if err != nil {
		t.Fatalf("history cap: %v", err)
	}
	if historyMonthKey != "2026-02" {
		t.Fatalf("expected normalized history month key 2026-02, got %q", historyMonthKey)
	}
	if !reflect.DeepEqual(expectedHistory, history) {
		t.Fatalf("unexpected history: %+v", history)
	}
}

func TestCapServiceExpenseTotalByMonthAndCurrencyNormalizesInput(t *testing.T) {
	t.Parallel()

	var receivedMonth string
	var receivedCurrency string

	svc, err := NewCapService(&capRepoStub{
		setFn:                 func(context.Context, domain.CapSetInput) (domain.MonthlyCap, domain.MonthlyCapChange, error) { return domain.MonthlyCap{}, domain.MonthlyCapChange{}, nil },
		getByMonthFn:          func(context.Context, string) (domain.MonthlyCap, error) { return domain.MonthlyCap{}, nil },
		listChangesByMonthFn:  func(context.Context, string) ([]domain.MonthlyCapChange, error) { return nil, nil },
		expenseTotalByMonthCurFn: func(ctx context.Context, monthKey, currencyCode string) (int64, error) {
			receivedMonth = monthKey
			receivedCurrency = currencyCode
			return 70000, nil
		},
	})
	if err != nil {
		t.Fatalf("new cap service: %v", err)
	}

	total, err := svc.ExpenseTotalByMonthAndCurrency(context.Background(), "2026-02", " usd ")
	if err != nil {
		t.Fatalf("expense total: %v", err)
	}
	if total != 70000 {
		t.Fatalf("expected total 70000, got %d", total)
	}
	if receivedMonth != "2026-02" {
		t.Fatalf("expected normalized month 2026-02, got %q", receivedMonth)
	}
	if receivedCurrency != "USD" {
		t.Fatalf("expected normalized currency USD, got %q", receivedCurrency)
	}
}

func TestCapServiceSetRejectsInvalidMonth(t *testing.T) {
	t.Parallel()

	svc, err := NewCapService(&capRepoStub{
		setFn:                 func(context.Context, domain.CapSetInput) (domain.MonthlyCap, domain.MonthlyCapChange, error) { return domain.MonthlyCap{}, domain.MonthlyCapChange{}, nil },
		getByMonthFn:          func(context.Context, string) (domain.MonthlyCap, error) { return domain.MonthlyCap{}, nil },
		listChangesByMonthFn:  func(context.Context, string) ([]domain.MonthlyCapChange, error) { return nil, nil },
		expenseTotalByMonthCurFn: func(context.Context, string, string) (int64, error) {
			return 0, nil
		},
	})
	if err != nil {
		t.Fatalf("new cap service: %v", err)
	}

	_, _, err = svc.Set(context.Background(), domain.CapSetInput{
		MonthKey:     "2026-13",
		AmountMinor:  1,
		CurrencyCode: "USD",
	})
	if !errors.Is(err, domain.ErrInvalidMonthKey) {
		t.Fatalf("expected ErrInvalidMonthKey, got %v", err)
	}
}
