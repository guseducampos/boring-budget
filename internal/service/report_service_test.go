package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"budgetto/internal/domain"
)

type reportEntryReaderStub struct {
	listFn func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error)
}

func (s *reportEntryReaderStub) List(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
	return s.listFn(ctx, filter)
}

type reportCapReaderStub struct {
	showFn         func(ctx context.Context, monthKey string) (domain.MonthlyCap, error)
	historyFn      func(ctx context.Context, monthKey string) ([]domain.MonthlyCapChange, error)
	expenseTotalFn func(ctx context.Context, monthKey, currencyCode string) (int64, error)
}

type reportFXConverterStub struct {
	convertFn func(ctx context.Context, amountMinor int64, fromCurrency, toCurrency, transactionDateUTC string) (domain.ConvertedAmount, error)
}

type reportSettingsReaderStub struct {
	getFn func(ctx context.Context) (domain.Settings, error)
}

func (s *reportFXConverterStub) Convert(ctx context.Context, amountMinor int64, fromCurrency, toCurrency, transactionDateUTC string) (domain.ConvertedAmount, error) {
	return s.convertFn(ctx, amountMinor, fromCurrency, toCurrency, transactionDateUTC)
}

func (s *reportSettingsReaderStub) Get(ctx context.Context) (domain.Settings, error) {
	return s.getFn(ctx)
}

func (s *reportCapReaderStub) Show(ctx context.Context, monthKey string) (domain.MonthlyCap, error) {
	return s.showFn(ctx, monthKey)
}

func (s *reportCapReaderStub) History(ctx context.Context, monthKey string) ([]domain.MonthlyCapChange, error) {
	return s.historyFn(ctx, monthKey)
}

func (s *reportCapReaderStub) ExpenseTotalByMonthAndCurrency(ctx context.Context, monthKey, currencyCode string) (int64, error) {
	return s.expenseTotalFn(ctx, monthKey, currencyCode)
}

func TestNewReportServiceRequiresEntryReader(t *testing.T) {
	t.Parallel()

	_, err := NewReportService(nil, nil)
	if err == nil {
		t.Fatalf("expected error for nil entry reader")
	}
}

func TestReportServiceGenerateAggregatesDeterministically(t *testing.T) {
	t.Parallel()

	catOne := int64(1)
	catTwo := int64(2)
	var capturedFilter domain.EntryListFilter

	svc, err := NewReportService(
		&reportEntryReaderStub{
			listFn: func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
				capturedFilter = filter
				return []domain.Entry{
					{ID: 2, Type: domain.EntryTypeExpense, AmountMinor: 3000, CurrencyCode: "USD", TransactionDateUTC: "2026-02-11T10:00:00Z"},
					{ID: 1, Type: domain.EntryTypeIncome, AmountMinor: 10000, CurrencyCode: "USD", TransactionDateUTC: "2026-02-10T09:00:00Z", CategoryID: &catOne},
					{ID: 3, Type: domain.EntryTypeExpense, AmountMinor: 2000, CurrencyCode: "EUR", TransactionDateUTC: "2026-02-11T11:00:00Z", CategoryID: &catTwo},
					{ID: 4, Type: domain.EntryTypeIncome, AmountMinor: 5000, CurrencyCode: "EUR", TransactionDateUTC: "2026-02-12T11:00:00Z"},
					{ID: 5, Type: domain.EntryTypeExpense, AmountMinor: 1000, CurrencyCode: "USD", TransactionDateUTC: "2026-02-10T12:00:00Z", CategoryID: &catOne},
				}, nil
			},
		},
		&reportCapReaderStub{
			showFn: func(ctx context.Context, monthKey string) (domain.MonthlyCap, error) {
				if monthKey != "2026-02" {
					return domain.MonthlyCap{}, domain.ErrCapNotFound
				}
				return domain.MonthlyCap{
					MonthKey:     monthKey,
					AmountMinor:  3500,
					CurrencyCode: "USD",
				}, nil
			},
			historyFn: func(ctx context.Context, monthKey string) ([]domain.MonthlyCapChange, error) {
				if monthKey != "2026-02" {
					return nil, nil
				}
				return []domain.MonthlyCapChange{
					{ID: 2, MonthKey: monthKey, NewAmountMinor: 3500, CurrencyCode: "USD", ChangedAtUTC: "2026-02-11T00:00:00Z"},
					{ID: 1, MonthKey: monthKey, NewAmountMinor: 3000, CurrencyCode: "USD", ChangedAtUTC: "2026-02-01T00:00:00Z"},
				}, nil
			},
			expenseTotalFn: func(ctx context.Context, monthKey, currencyCode string) (int64, error) {
				if monthKey != "2026-02" || currencyCode != "USD" {
					t.Fatalf("unexpected cap spend lookup %s/%s", monthKey, currencyCode)
				}
				return 4000, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("new report service: %v", err)
	}

	result, err := svc.Generate(context.Background(), ReportRequest{
		Period: domain.ReportPeriodInput{
			Scope:    domain.ReportScopeMonthly,
			MonthKey: "2026-02",
		},
		Grouping:  domain.ReportGroupingDay,
		LabelIDs:  []int64{9, 3, 9},
		LabelMode: "any",
	})
	if err != nil {
		t.Fatalf("generate report: %v", err)
	}
	report := result.Report

	if capturedFilter.DateFromUTC != "2026-02-01T00:00:00Z" {
		t.Fatalf("unexpected date_from filter: %q", capturedFilter.DateFromUTC)
	}
	if capturedFilter.DateToUTC != "2026-02-28T23:59:59.999999999Z" {
		t.Fatalf("unexpected date_to filter: %q", capturedFilter.DateToUTC)
	}
	if capturedFilter.LabelMode != domain.LabelFilterModeAny {
		t.Fatalf("expected label mode ANY, got %q", capturedFilter.LabelMode)
	}
	if !reflect.DeepEqual(capturedFilter.LabelIDs, []int64{3, 9}) {
		t.Fatalf("expected normalized labels [3 9], got %v", capturedFilter.LabelIDs)
	}

	if !reflect.DeepEqual(report.Earnings.ByCurrency, []domain.CurrencyTotal{
		{CurrencyCode: "EUR", TotalMinor: 5000},
		{CurrencyCode: "USD", TotalMinor: 10000},
	}) {
		t.Fatalf("unexpected earnings by currency: %+v", report.Earnings.ByCurrency)
	}
	if !reflect.DeepEqual(report.Spending.ByCurrency, []domain.CurrencyTotal{
		{CurrencyCode: "EUR", TotalMinor: 2000},
		{CurrencyCode: "USD", TotalMinor: 4000},
	}) {
		t.Fatalf("unexpected spending by currency: %+v", report.Spending.ByCurrency)
	}
	if !reflect.DeepEqual(report.Net.ByCurrency, []domain.CurrencyTotal{
		{CurrencyCode: "EUR", TotalMinor: 3000},
		{CurrencyCode: "USD", TotalMinor: 6000},
	}) {
		t.Fatalf("unexpected net by currency: %+v", report.Net.ByCurrency)
	}

	if !reflect.DeepEqual(report.Earnings.Groups, []domain.GroupTotal{
		{PeriodKey: "2026-02-10", CurrencyCode: "USD", TotalMinor: 10000},
		{PeriodKey: "2026-02-12", CurrencyCode: "EUR", TotalMinor: 5000},
	}) {
		t.Fatalf("unexpected earnings groups: %+v", report.Earnings.Groups)
	}
	if !reflect.DeepEqual(report.Spending.Groups, []domain.GroupTotal{
		{PeriodKey: "2026-02-10", CurrencyCode: "USD", TotalMinor: 1000},
		{PeriodKey: "2026-02-11", CurrencyCode: "EUR", TotalMinor: 2000},
		{PeriodKey: "2026-02-11", CurrencyCode: "USD", TotalMinor: 3000},
	}) {
		t.Fatalf("unexpected spending groups: %+v", report.Spending.Groups)
	}

	if len(report.Spending.Categories) != 3 {
		t.Fatalf("expected 3 spending categories, got %+v", report.Spending.Categories)
	}
	if report.Spending.Categories[0].CategoryKey != domain.CategoryOrphanKey || report.Spending.Categories[0].TotalMinor != 3000 {
		t.Fatalf("expected orphan bucket first in spending categories, got %+v", report.Spending.Categories[0])
	}

	if len(report.CapStatus) != 1 {
		t.Fatalf("expected one cap status, got %+v", report.CapStatus)
	}
	if report.CapStatus[0].OverspendMinor != 500 || !report.CapStatus[0].IsExceeded {
		t.Fatalf("unexpected cap status %+v", report.CapStatus[0])
	}

	if len(report.CapChanges) != 2 {
		t.Fatalf("expected two cap changes, got %+v", report.CapChanges)
	}
	if report.CapChanges[0].ID != 1 || report.CapChanges[1].ID != 2 {
		t.Fatalf("expected cap changes sorted by changed_at/id, got %+v", report.CapChanges)
	}

	if len(result.Warnings) != 1 {
		t.Fatalf("expected one warning for orphan spending threshold, got %+v", result.Warnings)
	}
	if result.Warnings[0].Code != domain.WarningCodeOrphanSpendingExceeded {
		t.Fatalf("expected orphan spending warning code, got %+v", result.Warnings[0])
	}
}

func TestReportServiceGenerateSkipsMissingCap(t *testing.T) {
	t.Parallel()

	svc, err := NewReportService(
		&reportEntryReaderStub{
			listFn: func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
				return []domain.Entry{
					{ID: 1, Type: domain.EntryTypeExpense, AmountMinor: 1000, CurrencyCode: "USD", TransactionDateUTC: "2026-02-10T00:00:00Z"},
				}, nil
			},
		},
		&reportCapReaderStub{
			showFn: func(ctx context.Context, monthKey string) (domain.MonthlyCap, error) {
				return domain.MonthlyCap{}, domain.ErrCapNotFound
			},
			historyFn: func(ctx context.Context, monthKey string) ([]domain.MonthlyCapChange, error) {
				return nil, nil
			},
			expenseTotalFn: func(ctx context.Context, monthKey, currencyCode string) (int64, error) {
				return 0, errors.New("should not be called")
			},
		},
	)
	if err != nil {
		t.Fatalf("new report service: %v", err)
	}

	result, err := svc.Generate(context.Background(), ReportRequest{
		Period: domain.ReportPeriodInput{
			Scope:    domain.ReportScopeMonthly,
			MonthKey: "2026-02",
		},
	})
	if err != nil {
		t.Fatalf("generate report: %v", err)
	}
	report := result.Report

	if len(report.CapStatus) != 0 {
		t.Fatalf("expected no cap status, got %+v", report.CapStatus)
	}

	if len(result.Warnings) != 1 || result.Warnings[0].Code != domain.WarningCodeOrphanSpendingExceeded {
		t.Fatalf("expected orphan spending warning for uncategorized expense, got %+v", result.Warnings)
	}
}

func TestReportServiceGenerateSupportsConvertedSummary(t *testing.T) {
	t.Parallel()

	svc, err := NewReportService(
		&reportEntryReaderStub{
			listFn: func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
				return []domain.Entry{
					{ID: 1, Type: domain.EntryTypeIncome, AmountMinor: 1000, CurrencyCode: "USD", TransactionDateUTC: "2026-02-01T00:00:00Z"},
					{ID: 2, Type: domain.EntryTypeExpense, AmountMinor: 500, CurrencyCode: "EUR", TransactionDateUTC: "2026-03-01T00:00:00Z"},
				}, nil
			},
		},
		nil,
		WithReportFXConverter(&reportFXConverterStub{
			convertFn: func(ctx context.Context, amountMinor int64, fromCurrency, toCurrency, transactionDateUTC string) (domain.ConvertedAmount, error) {
				isEstimate := transactionDateUTC == "2026-03-01T00:00:00Z"
				return domain.ConvertedAmount{
					AmountMinor: amountMinor,
					Snapshot: domain.FXRateSnapshot{
						Provider:      "stub",
						BaseCurrency:  fromCurrency,
						QuoteCurrency: toCurrency,
						Rate:          "1",
						RateDate:      "2026-02-01",
						IsEstimate:    isEstimate,
						FetchedAtUTC:  "2026-02-01T00:00:00Z",
					},
				}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("new report service: %v", err)
	}

	result, err := svc.Generate(context.Background(), ReportRequest{
		Period: domain.ReportPeriodInput{
			Scope:    domain.ReportScopeBimonthly,
			MonthKey: "2026-02",
		},
		ConvertTo: "USD",
	})
	if err != nil {
		t.Fatalf("generate converted report: %v", err)
	}

	if result.Report.Converted == nil {
		t.Fatalf("expected converted summary")
	}
	if result.Report.Converted.EarningsMinor != 1000 || result.Report.Converted.SpendingMinor != 500 || result.Report.Converted.NetMinor != 500 {
		t.Fatalf("unexpected converted summary: %+v", result.Report.Converted)
	}

	foundFXWarning := false
	for _, warning := range result.Warnings {
		if warning.Code == domain.WarningCodeFXEstimateUsed {
			foundFXWarning = true
		}
	}
	if !foundFXWarning {
		t.Fatalf("expected FX_ESTIMATE_USED warning, got %+v", result.Warnings)
	}
}

func TestReportServiceGenerateUsesSettingsThresholdOverrides(t *testing.T) {
	t.Parallel()

	svc, err := NewReportService(
		&reportEntryReaderStub{
			listFn: func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
				return []domain.Entry{
					{ID: 1, Type: domain.EntryTypeExpense, AmountMinor: 1000, CurrencyCode: "USD", TransactionDateUTC: "2026-02-01T00:00:00Z"},
					{ID: 2, Type: domain.EntryTypeExpense, AmountMinor: 1000, CurrencyCode: "USD", TransactionDateUTC: "2026-02-02T00:00:00Z"},
				}, nil
			},
		},
		nil,
		WithReportSettingsReader(&reportSettingsReaderStub{
			getFn: func(ctx context.Context) (domain.Settings, error) {
				return domain.Settings{
					OrphanCountThreshold:       10,
					OrphanSpendingThresholdBPS: 10001,
				}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("new report service: %v", err)
	}

	result, err := svc.Generate(context.Background(), ReportRequest{
		Period: domain.ReportPeriodInput{
			Scope:    domain.ReportScopeMonthly,
			MonthKey: "2026-02",
		},
	})
	if err != nil {
		t.Fatalf("generate report with settings thresholds: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("expected no orphan warnings with overridden thresholds, got %+v", result.Warnings)
	}
}

func TestReportServiceGenerateReturnsErrorWhenSettingsReadFails(t *testing.T) {
	t.Parallel()

	svc, err := NewReportService(
		&reportEntryReaderStub{
			listFn: func(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
				return []domain.Entry{
					{ID: 1, Type: domain.EntryTypeExpense, AmountMinor: 1000, CurrencyCode: "USD", TransactionDateUTC: "2026-02-01T00:00:00Z"},
				}, nil
			},
		},
		nil,
		WithReportSettingsReader(&reportSettingsReaderStub{
			getFn: func(ctx context.Context) (domain.Settings, error) {
				return domain.Settings{}, errors.New("settings db unavailable")
			},
		}),
	)
	if err != nil {
		t.Fatalf("new report service: %v", err)
	}

	_, err = svc.Generate(context.Background(), ReportRequest{
		Period: domain.ReportPeriodInput{
			Scope:    domain.ReportScopeMonthly,
			MonthKey: "2026-02",
		},
	})
	if err == nil {
		t.Fatalf("expected settings read error")
	}
}
