package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"budgetto/internal/domain"
)

type BalanceEntryReader interface {
	List(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error)
}

type BalanceService struct {
	entryReader BalanceEntryReader
	fxConverter BalanceFXConverter
}

type BalanceRequest struct {
	IncludeLifetime bool
	IncludeRange    bool
	RangeFromUTC    string
	RangeToUTC      string
	CategoryID      *int64
	LabelIDs        []int64
	LabelMode       string
	ConvertTo       string
}

type BalanceFXConverter interface {
	Convert(ctx context.Context, amountMinor int64, fromCurrency, toCurrency, transactionDateUTC string) (domain.ConvertedAmount, error)
}

type BalanceServiceOption func(*BalanceService)

func WithBalanceFXConverter(converter BalanceFXConverter) BalanceServiceOption {
	return func(s *BalanceService) {
		s.fxConverter = converter
	}
}

func NewBalanceService(entryReader BalanceEntryReader, opts ...BalanceServiceOption) (*BalanceService, error) {
	if entryReader == nil {
		return nil, fmt.Errorf("balance service: entry reader is required")
	}

	service := &BalanceService{entryReader: entryReader}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}

	return service, nil
}

func (s *BalanceService) Compute(ctx context.Context, req BalanceRequest) (domain.BalanceViews, error) {
	if err := domain.ValidateOptionalCategoryID(req.CategoryID); err != nil {
		return domain.BalanceViews{}, err
	}

	normalizedLabelIDs, err := domain.NormalizeLabelIDs(req.LabelIDs)
	if err != nil {
		return domain.BalanceViews{}, err
	}

	normalizedLabelMode, err := domain.NormalizeLabelMode(req.LabelMode)
	if err != nil {
		return domain.BalanceViews{}, err
	}

	fromUTC, err := normalizeRangeBoundary(req.RangeFromUTC, false)
	if err != nil {
		return domain.BalanceViews{}, err
	}
	toUTC, err := normalizeRangeBoundary(req.RangeToUTC, true)
	if err != nil {
		return domain.BalanceViews{}, err
	}
	if err := domain.ValidateDateRange(fromUTC, toUTC); err != nil {
		return domain.BalanceViews{}, err
	}

	includeLifetime := req.IncludeLifetime
	includeRange := req.IncludeRange
	if !includeLifetime && !includeRange {
		includeLifetime = true
		includeRange = true
	}

	views := domain.BalanceViews{}
	targetCurrency := strings.TrimSpace(req.ConvertTo)
	if targetCurrency != "" {
		normalizedTarget, err := domain.NormalizeCurrencyCode(targetCurrency)
		if err != nil {
			return domain.BalanceViews{}, err
		}
		targetCurrency = normalizedTarget
		if s.fxConverter == nil {
			return domain.BalanceViews{}, domain.ErrFXRateUnavailable
		}
	}

	if includeLifetime {
		lifetimeFilter := domain.EntryListFilter{
			CategoryID: req.CategoryID,
			LabelIDs:   normalizedLabelIDs,
			LabelMode:  normalizedLabelMode,
		}

		netByCurrency, err := s.netByCurrency(ctx, lifetimeFilter)
		if err != nil {
			return domain.BalanceViews{}, err
		}
		views.Lifetime = &domain.BalanceView{ByCurrency: netByCurrency}

		if targetCurrency != "" {
			converted, usedEstimate, err := s.convertNetByFilter(ctx, lifetimeFilter, targetCurrency)
			if err != nil {
				return domain.BalanceViews{}, err
			}
			views.LifetimeConverted = &domain.ConvertedBalanceView{
				TargetCurrency:   targetCurrency,
				NetMinor:         converted,
				UsedEstimateRate: usedEstimate,
			}
		}
	}

	if includeRange {
		rangeFilter := domain.EntryListFilter{
			CategoryID:  req.CategoryID,
			DateFromUTC: fromUTC,
			DateToUTC:   toUTC,
			LabelIDs:    normalizedLabelIDs,
			LabelMode:   normalizedLabelMode,
		}

		netByCurrency, err := s.netByCurrency(ctx, rangeFilter)
		if err != nil {
			return domain.BalanceViews{}, err
		}
		views.Range = &domain.BalanceView{ByCurrency: netByCurrency}

		if targetCurrency != "" {
			converted, usedEstimate, err := s.convertNetByFilter(ctx, rangeFilter, targetCurrency)
			if err != nil {
				return domain.BalanceViews{}, err
			}
			views.RangeConverted = &domain.ConvertedBalanceView{
				TargetCurrency:   targetCurrency,
				NetMinor:         converted,
				UsedEstimateRate: usedEstimate,
			}
		}
	}

	return views, nil
}

func (s *BalanceService) netByCurrency(ctx context.Context, filter domain.EntryListFilter) ([]domain.CurrencyNet, error) {
	entries, err := s.entryReader.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	totals := map[string]int64{}
	for _, entry := range entries {
		switch entry.Type {
		case domain.EntryTypeIncome:
			totals[entry.CurrencyCode] += entry.AmountMinor
		case domain.EntryTypeExpense:
			totals[entry.CurrencyCode] -= entry.AmountMinor
		}
	}

	currencies := make([]string, 0, len(totals))
	for currency := range totals {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)

	output := make([]domain.CurrencyNet, 0, len(currencies))
	for _, currency := range currencies {
		output = append(output, domain.CurrencyNet{
			CurrencyCode: currency,
			NetMinor:     totals[currency],
		})
	}
	return output, nil
}

func normalizeRangeBoundary(raw string, endOfDay bool) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UTC().Format(time.RFC3339Nano), nil
		}
	}

	dateOnly, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return "", domain.ErrInvalidTransactionDate
	}

	if endOfDay {
		dateOnly = dateOnly.Add(24*time.Hour - time.Nanosecond)
	}

	return dateOnly.UTC().Format(time.RFC3339Nano), nil
}

func (s *BalanceService) convertNetByFilter(ctx context.Context, filter domain.EntryListFilter, targetCurrency string) (int64, bool, error) {
	entries, err := s.entryReader.List(ctx, filter)
	if err != nil {
		return 0, false, err
	}

	netMinor := int64(0)
	usedEstimate := false
	for _, entry := range entries {
		converted, err := s.fxConverter.Convert(ctx, entry.AmountMinor, entry.CurrencyCode, targetCurrency, entry.TransactionDateUTC)
		if err != nil {
			return 0, false, err
		}

		if converted.Snapshot.IsEstimate {
			usedEstimate = true
		}

		switch entry.Type {
		case domain.EntryTypeIncome:
			netMinor += converted.AmountMinor
		case domain.EntryTypeExpense:
			netMinor -= converted.AmountMinor
		}
	}

	return netMinor, usedEstimate, nil
}
