package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"boring-budget/internal/domain"
	"boring-budget/internal/reporting"
)

type ReportEntryReader interface {
	List(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error)
}

type ReportCapReader interface {
	Show(ctx context.Context, monthKey string) (domain.MonthlyCap, error)
	History(ctx context.Context, monthKey string) ([]domain.MonthlyCapChange, error)
	ExpenseTotalByMonthAndCurrency(ctx context.Context, monthKey, currencyCode string) (int64, error)
}

type ReportCategoryReader interface {
	List(ctx context.Context) ([]domain.Category, error)
}

type ReportCategoryByIDsReader interface {
	ListByIDs(ctx context.Context, ids []int64) ([]domain.Category, error)
}

type ReportService struct {
	entryReader    ReportEntryReader
	capReader      ReportCapReader
	fxConverter    ReportFXConverter
	settingsReader ReportSettingsReader
	categoryReader ReportCategoryReader
	cardDebtReader ReportCardDebtReader
}

type ReportRequest struct {
	Period              domain.ReportPeriodInput
	Grouping            string
	CategoryID          *int64
	LabelIDs            []int64
	LabelMode           string
	ConvertTo           string
	PaymentMethod       string
	PaymentCardID       *int64
	PaymentCardNickname string
	PaymentCardLookup   string
}

type ReportResult struct {
	Report   domain.Report
	Warnings []domain.Warning
}

type ReportFXConverter interface {
	Convert(ctx context.Context, amountMinor int64, fromCurrency, toCurrency, transactionDateUTC string) (domain.ConvertedAmount, error)
}

type ReportSettingsReader interface {
	Get(ctx context.Context) (domain.Settings, error)
}

type ReportCardDebtReader interface {
	ShowDebtAll(ctx context.Context) ([]CardDebtCardSummary, error)
}

type ReportServiceOption func(*ReportService)

func WithReportFXConverter(converter ReportFXConverter) ReportServiceOption {
	return func(s *ReportService) {
		s.fxConverter = converter
	}
}

func WithReportSettingsReader(reader ReportSettingsReader) ReportServiceOption {
	return func(s *ReportService) {
		s.settingsReader = reader
	}
}

func WithReportCategoryReader(reader ReportCategoryReader) ReportServiceOption {
	return func(s *ReportService) {
		s.categoryReader = reader
	}
}

func WithReportCardDebtReader(reader ReportCardDebtReader) ReportServiceOption {
	return func(s *ReportService) {
		s.cardDebtReader = reader
	}
}

func NewReportService(entryReader ReportEntryReader, capReader ReportCapReader, opts ...ReportServiceOption) (*ReportService, error) {
	if entryReader == nil {
		return nil, fmt.Errorf("report service: entry reader is required")
	}

	service := &ReportService{
		entryReader: entryReader,
		capReader:   capReader,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}

	return service, nil
}

func (s *ReportService) Generate(ctx context.Context, req ReportRequest) (ReportResult, error) {
	period, err := domain.BuildReportPeriod(req.Period)
	if err != nil {
		return ReportResult{}, err
	}

	grouping, err := domain.NormalizeReportGrouping(req.Grouping)
	if err != nil {
		return ReportResult{}, err
	}

	if err := domain.ValidateOptionalCategoryID(req.CategoryID); err != nil {
		return ReportResult{}, err
	}

	normalizedLabelIDs, err := domain.NormalizeLabelIDs(req.LabelIDs)
	if err != nil {
		return ReportResult{}, err
	}

	normalizedLabelMode, err := domain.NormalizeLabelMode(req.LabelMode)
	if err != nil {
		return ReportResult{}, err
	}
	normalizedPaymentMethod, err := domain.NormalizePaymentMethodFilter(req.PaymentMethod)
	if err != nil {
		return ReportResult{}, err
	}
	if err := domain.ValidateCardSelector(req.PaymentCardID, req.PaymentCardNickname, req.PaymentCardLookup); err != nil {
		return ReportResult{}, err
	}
	if normalizedPaymentMethod == domain.PaymentMethodCash && domain.HasCardSelector(req.PaymentCardID, req.PaymentCardNickname, req.PaymentCardLookup) {
		return ReportResult{}, domain.ErrCardNotAllowed
	}

	entries, err := s.entryReader.List(ctx, domain.EntryListFilter{
		CategoryID:          req.CategoryID,
		DateFromUTC:         period.FromUTC,
		DateToUTC:           period.ToUTC,
		LabelIDs:            normalizedLabelIDs,
		LabelMode:           normalizedLabelMode,
		PaymentMethod:       normalizedPaymentMethod,
		PaymentCardID:       req.PaymentCardID,
		PaymentCardNickname: strings.TrimSpace(req.PaymentCardNickname),
		PaymentCardLookup:   strings.TrimSpace(req.PaymentCardLookup),
	})
	if err != nil {
		return ReportResult{}, err
	}

	reporting.SortEntriesDeterministic(entries)

	categoryLabelResolver, err := s.buildCategoryLabelResolver(ctx, entries)
	if err != nil {
		return ReportResult{}, err
	}

	aggregate, err := reporting.BuildAggregate(entries, grouping, categoryLabelResolver)
	if err != nil {
		return ReportResult{}, err
	}

	report := domain.Report{
		Period:         period,
		Grouping:       grouping,
		Earnings:       aggregate.Earnings,
		Spending:       aggregate.Spending,
		Net:            aggregate.Net,
		PaymentMethods: nil,
		CapStatus:      []domain.ReportCapStatus{},
		CapChanges:     []domain.MonthlyCapChange{},
	}
	paymentMethods := aggregate.PaymentMethods
	if s.cardDebtReader != nil {
		cardDebts, err := s.cardDebtReader.ShowDebtAll(ctx)
		if err != nil {
			return ReportResult{}, err
		}
		paymentMethods.CreditLiability = toReportCardLiability(cardDebts)
	}
	report.PaymentMethods = &paymentMethods

	conversionWarnings := []domain.Warning{}
	targetCurrency := strings.TrimSpace(req.ConvertTo)
	if targetCurrency != "" {
		if s.fxConverter == nil {
			return ReportResult{}, domain.ErrFXRateUnavailable
		}

		normalizedTarget, err := domain.NormalizeCurrencyCode(targetCurrency)
		if err != nil {
			return ReportResult{}, err
		}

		convertedSummary, usedEstimate, err := s.buildConvertedSummary(ctx, entries, normalizedTarget)
		if err != nil {
			return ReportResult{}, err
		}
		report.Converted = &convertedSummary
		if usedEstimate {
			conversionWarnings = append(conversionWarnings, domain.Warning{
				Code:    domain.WarningCodeFXEstimateUsed,
				Message: domain.FXEstimateWarningMessage,
				Details: map[string]any{
					"target_currency": normalizedTarget,
				},
			})
		}
	}

	if s.capReader != nil {
		statuses, changes, err := s.buildCapData(ctx, period)
		if err != nil {
			return ReportResult{}, err
		}
		report.CapStatus = statuses
		report.CapChanges = changes
	}

	orphanCountThreshold := domain.DefaultOrphanCountThreshold
	orphanSpendingThresholdBPS := domain.DefaultOrphanSpendingThresholdBPS
	if s.settingsReader != nil {
		settings, err := s.settingsReader.Get(ctx)
		if err == nil {
			if settings.OrphanCountThreshold > 0 {
				orphanCountThreshold = int(settings.OrphanCountThreshold)
			}
			if settings.OrphanSpendingThresholdBPS > 0 {
				orphanSpendingThresholdBPS = int(settings.OrphanSpendingThresholdBPS)
			}
		} else if !errors.Is(err, domain.ErrSettingsNotFound) {
			return ReportResult{}, err
		}
	}

	warnings, err := s.buildOrphanWarnings(entries, period, report.CapStatus, orphanCountThreshold, orphanSpendingThresholdBPS)
	if err != nil {
		return ReportResult{}, err
	}
	warnings = append(warnings, conversionWarnings...)

	return ReportResult{Report: report, Warnings: warnings}, nil
}

func toReportCardLiability(rows []CardDebtCardSummary) []domain.ReportCardLiability {
	if len(rows) == 0 {
		return []domain.ReportCardLiability{}
	}

	out := make([]domain.ReportCardLiability, 0, len(rows))
	for _, row := range rows {
		for _, bucket := range row.Buckets {
			out = append(out, domain.ReportCardLiability{
				CardID:             row.Card.ID,
				CardNickname:       row.Card.Nickname,
				CurrencyCode:       bucket.CurrencyCode,
				BalanceMinorSigned: bucket.BalanceMinorSigned,
				State:              bucket.State,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].CardID != out[j].CardID {
			return out[i].CardID < out[j].CardID
		}
		return out[i].CurrencyCode < out[j].CurrencyCode
	})

	return out
}

func (s *ReportService) buildCategoryLabelResolver(ctx context.Context, entries []domain.Entry) (reporting.CategoryLabelResolver, error) {
	categoryIDs := map[int64]struct{}{}
	for _, entry := range entries {
		if entry.CategoryID != nil {
			categoryIDs[*entry.CategoryID] = struct{}{}
		}
	}

	if len(categoryIDs) == 0 {
		return nil, nil
	}

	neededCategoryIDs := mapKeysSorted(categoryIDs)
	categoryNames := map[int64]string{}
	if s.categoryReader != nil {
		var (
			categories []domain.Category
			err        error
		)

		if byIDsReader, ok := s.categoryReader.(ReportCategoryByIDsReader); ok {
			categories, err = byIDsReader.ListByIDs(ctx, neededCategoryIDs)
		} else {
			categories, err = s.categoryReader.List(ctx)
		}
		if err != nil {
			return nil, err
		}

		for _, category := range categories {
			if _, needed := categoryIDs[category.ID]; !needed {
				continue
			}
			if trimmedName := strings.TrimSpace(category.Name); trimmedName != "" {
				categoryNames[category.ID] = trimmedName
			}
		}
	}

	return func(categoryID int64) string {
		if name, ok := categoryNames[categoryID]; ok {
			return name
		}
		return domain.CategoryUnknownLabel
	}, nil
}

func mapKeysSorted(values map[int64]struct{}) []int64 {
	keys := make([]int64, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}

	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	return keys
}

func (s *ReportService) buildConvertedSummary(ctx context.Context, entries []domain.Entry, targetCurrency string) (domain.ConvertedSummary, bool, error) {
	converted := domain.ConvertedSummary{
		TargetCurrency: targetCurrency,
	}
	usedEstimate := false

	for _, entry := range entries {
		amount, err := s.fxConverter.Convert(ctx, entry.AmountMinor, entry.CurrencyCode, targetCurrency, entry.TransactionDateUTC)
		if err != nil {
			return domain.ConvertedSummary{}, false, err
		}

		if amount.Snapshot.IsEstimate {
			usedEstimate = true
		}

		switch entry.Type {
		case domain.EntryTypeIncome:
			converted.EarningsMinor += amount.AmountMinor
			converted.NetMinor += amount.AmountMinor
		case domain.EntryTypeExpense:
			converted.SpendingMinor += amount.AmountMinor
			converted.NetMinor -= amount.AmountMinor
		}
	}

	converted.UsedEstimateRate = usedEstimate
	return converted, usedEstimate, nil
}

func (s *ReportService) buildCapData(ctx context.Context, period domain.ReportPeriod) ([]domain.ReportCapStatus, []domain.MonthlyCapChange, error) {
	monthKeys, err := domain.MonthKeysInPeriod(period.FromUTC, period.ToUTC)
	if err != nil {
		return nil, nil, err
	}

	statuses := make([]domain.ReportCapStatus, 0, len(monthKeys))
	allChanges := []domain.MonthlyCapChange{}

	for _, monthKey := range monthKeys {
		changes, err := s.capReader.History(ctx, monthKey)
		if err != nil {
			return nil, nil, err
		}
		allChanges = append(allChanges, changes...)

		capValue, err := s.capReader.Show(ctx, monthKey)
		if err != nil {
			if errors.Is(err, domain.ErrCapNotFound) {
				continue
			}
			return nil, nil, err
		}

		totalSpend, err := s.capReader.ExpenseTotalByMonthAndCurrency(ctx, monthKey, capValue.CurrencyCode)
		if err != nil {
			return nil, nil, err
		}

		overspend := totalSpend - capValue.AmountMinor
		if overspend < 0 {
			overspend = 0
		}

		statuses = append(statuses, domain.ReportCapStatus{
			MonthKey:        monthKey,
			CurrencyCode:    capValue.CurrencyCode,
			CapAmountMinor:  capValue.AmountMinor,
			SpendTotalMinor: totalSpend,
			OverspendMinor:  overspend,
			IsExceeded:      overspend > 0,
		})
	}

	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].MonthKey != statuses[j].MonthKey {
			return statuses[i].MonthKey < statuses[j].MonthKey
		}
		return statuses[i].CurrencyCode < statuses[j].CurrencyCode
	})

	sort.Slice(allChanges, func(i, j int) bool {
		if allChanges[i].MonthKey != allChanges[j].MonthKey {
			return allChanges[i].MonthKey < allChanges[j].MonthKey
		}
		if allChanges[i].ChangedAtUTC != allChanges[j].ChangedAtUTC {
			return allChanges[i].ChangedAtUTC < allChanges[j].ChangedAtUTC
		}
		return allChanges[i].ID < allChanges[j].ID
	})

	return statuses, allChanges, nil
}

type orphanSpendKey struct {
	MonthKey     string
	CurrencyCode string
}

type orphanSpendStats struct {
	OrphanSpendMinor int64
	MonthSpendMinor  int64
}

func (s *ReportService) buildOrphanWarnings(entries []domain.Entry, period domain.ReportPeriod, capStatus []domain.ReportCapStatus, countThreshold int, spendingThresholdBPS int) ([]domain.Warning, error) {
	warnings := make([]domain.Warning, 0)

	orphanCount := 0
	orphanSpendByMonthCurrency := map[orphanSpendKey]orphanSpendStats{}
	for _, entry := range entries {
		if entry.CategoryID != nil {
			continue
		}

		orphanCount++
		if entry.Type != domain.EntryTypeExpense {
			continue
		}

		monthKey, err := domain.MonthKeyFromDateTimeUTC(entry.TransactionDateUTC)
		if err != nil {
			return nil, err
		}

		key := orphanSpendKey{MonthKey: monthKey, CurrencyCode: entry.CurrencyCode}
		stats := orphanSpendByMonthCurrency[key]
		stats.OrphanSpendMinor += entry.AmountMinor
		orphanSpendByMonthCurrency[key] = stats
	}

	for _, entry := range entries {
		if entry.Type != domain.EntryTypeExpense {
			continue
		}
		monthKey, err := domain.MonthKeyFromDateTimeUTC(entry.TransactionDateUTC)
		if err != nil {
			return nil, err
		}
		key := orphanSpendKey{MonthKey: monthKey, CurrencyCode: entry.CurrencyCode}
		stats := orphanSpendByMonthCurrency[key]
		stats.MonthSpendMinor += entry.AmountMinor
		orphanSpendByMonthCurrency[key] = stats
	}

	if orphanCount > countThreshold {
		warnings = append(warnings, domain.Warning{
			Code:    domain.WarningCodeOrphanCountExceeded,
			Message: domain.OrphanCountWarningMessage,
			Details: domain.OrphanCountWarningDetails{
				PeriodFromUTC: period.FromUTC,
				PeriodToUTC:   period.ToUTC,
				OrphanCount:   orphanCount,
				Threshold:     countThreshold,
			},
		})
	}

	caps := make(map[orphanSpendKey]domain.ReportCapStatus, len(capStatus))
	for _, capItem := range capStatus {
		caps[orphanSpendKey{MonthKey: capItem.MonthKey, CurrencyCode: capItem.CurrencyCode}] = capItem
	}

	keys := make([]orphanSpendKey, 0, len(orphanSpendByMonthCurrency))
	for key := range orphanSpendByMonthCurrency {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].MonthKey != keys[j].MonthKey {
			return keys[i].MonthKey < keys[j].MonthKey
		}
		return keys[i].CurrencyCode < keys[j].CurrencyCode
	})

	for _, key := range keys {
		stats := orphanSpendByMonthCurrency[key]
		if stats.OrphanSpendMinor == 0 {
			continue
		}

		exceededByMonthSpend := stats.MonthSpendMinor > 0 && (stats.OrphanSpendMinor*10000 > int64(spendingThresholdBPS)*stats.MonthSpendMinor)
		exceededByCap := false
		capAmountMinor := int64(0)
		if capItem, ok := caps[key]; ok {
			capAmountMinor = capItem.CapAmountMinor
			if capAmountMinor > 0 {
				exceededByCap = stats.OrphanSpendMinor*10000 > int64(spendingThresholdBPS)*capAmountMinor
			}
		}

		if !exceededByMonthSpend && !exceededByCap {
			continue
		}

		triggeredBy := []string{}
		if exceededByMonthSpend {
			triggeredBy = append(triggeredBy, "MONTH_SPEND")
		}
		if exceededByCap {
			triggeredBy = append(triggeredBy, "MONTH_CAP")
		}

		var capAmountPtr *int64
		if capAmountMinor > 0 {
			capValue := capAmountMinor
			capAmountPtr = &capValue
		}

		warnings = append(warnings, domain.Warning{
			Code:    domain.WarningCodeOrphanSpendingExceeded,
			Message: domain.OrphanSpendingWarningMessage,
			Details: domain.OrphanSpendingWarningDetails{
				MonthKey:       key.MonthKey,
				CurrencyCode:   key.CurrencyCode,
				OrphanSpend:    stats.OrphanSpendMinor,
				MonthSpend:     stats.MonthSpendMinor,
				CapAmount:      capAmountPtr,
				ThresholdBPS:   spendingThresholdBPS,
				TriggeredBy:    triggeredBy,
				RatioToSpendBP: ratioBPS(stats.OrphanSpendMinor, stats.MonthSpendMinor),
				RatioToCapBP:   ratioBPS(stats.OrphanSpendMinor, capAmountMinor),
			},
		})
	}

	return warnings, nil
}

func ratioBPS(part, whole int64) int64 {
	if whole <= 0 {
		return 0
	}
	return (part * 10000) / whole
}
