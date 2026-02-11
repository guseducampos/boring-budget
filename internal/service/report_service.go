package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"budgetto/internal/domain"
)

type ReportEntryReader interface {
	List(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error)
}

type ReportCapReader interface {
	Show(ctx context.Context, monthKey string) (domain.MonthlyCap, error)
	History(ctx context.Context, monthKey string) ([]domain.MonthlyCapChange, error)
	ExpenseTotalByMonthAndCurrency(ctx context.Context, monthKey, currencyCode string) (int64, error)
}

type ReportService struct {
	entryReader ReportEntryReader
	capReader   ReportCapReader
}

type ReportRequest struct {
	Period     domain.ReportPeriodInput
	Grouping   string
	CategoryID *int64
	LabelIDs   []int64
	LabelMode  string
}

type ReportResult struct {
	Report   domain.Report
	Warnings []domain.Warning
}

func NewReportService(entryReader ReportEntryReader, capReader ReportCapReader) (*ReportService, error) {
	if entryReader == nil {
		return nil, fmt.Errorf("report service: entry reader is required")
	}

	return &ReportService{
		entryReader: entryReader,
		capReader:   capReader,
	}, nil
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

	entries, err := s.entryReader.List(ctx, domain.EntryListFilter{
		CategoryID:  req.CategoryID,
		DateFromUTC: period.FromUTC,
		DateToUTC:   period.ToUTC,
		LabelIDs:    normalizedLabelIDs,
		LabelMode:   normalizedLabelMode,
	})
	if err != nil {
		return ReportResult{}, err
	}

	sortEntriesDeterministic(entries)

	earnByCurrency := map[string]int64{}
	spendByCurrency := map[string]int64{}
	earnGroups := map[groupCurrencyKey]int64{}
	spendGroups := map[groupCurrencyKey]int64{}
	earnCategories := map[categoryCurrencyKey]int64{}
	spendCategories := map[categoryCurrencyKey]int64{}

	for _, entry := range entries {
		periodKey, err := domain.PeriodKeyForTransaction(entry.TransactionDateUTC, grouping)
		if err != nil {
			return ReportResult{}, err
		}

		switch entry.Type {
		case domain.EntryTypeIncome:
			earnByCurrency[entry.CurrencyCode] += entry.AmountMinor
			earnGroups[groupCurrencyKey{PeriodKey: periodKey, CurrencyCode: entry.CurrencyCode}] += entry.AmountMinor
			earnCategories[toCategoryCurrencyKey(entry)] += entry.AmountMinor
		case domain.EntryTypeExpense:
			spendByCurrency[entry.CurrencyCode] += entry.AmountMinor
			spendGroups[groupCurrencyKey{PeriodKey: periodKey, CurrencyCode: entry.CurrencyCode}] += entry.AmountMinor
			spendCategories[toCategoryCurrencyKey(entry)] += entry.AmountMinor
		}
	}

	report := domain.Report{
		Period:   period,
		Grouping: grouping,
		Earnings: domain.ReportSection{
			ByCurrency: mapCurrencyTotals(earnByCurrency),
			Groups:     mapGroupTotals(earnGroups),
			Categories: mapCategoryTotals(earnCategories),
		},
		Spending: domain.ReportSection{
			ByCurrency: mapCurrencyTotals(spendByCurrency),
			Groups:     mapGroupTotals(spendGroups),
			Categories: mapCategoryTotals(spendCategories),
		},
		Net: domain.ReportNet{
			ByCurrency: mapNetTotals(earnByCurrency, spendByCurrency),
		},
		CapStatus:  []domain.ReportCapStatus{},
		CapChanges: []domain.MonthlyCapChange{},
	}

	if s.capReader != nil {
		statuses, changes, err := s.buildCapData(ctx, period)
		if err != nil {
			return ReportResult{}, err
		}
		report.CapStatus = statuses
		report.CapChanges = changes
	}

	warnings, err := s.buildOrphanWarnings(entries, period, report.CapStatus)
	if err != nil {
		return ReportResult{}, err
	}

	return ReportResult{Report: report, Warnings: warnings}, nil
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

func (s *ReportService) buildOrphanWarnings(entries []domain.Entry, period domain.ReportPeriod, capStatus []domain.ReportCapStatus) ([]domain.Warning, error) {
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

	if orphanCount > domain.DefaultOrphanCountThreshold {
		warnings = append(warnings, domain.Warning{
			Code:    domain.WarningCodeOrphanCountExceeded,
			Message: domain.OrphanCountWarningMessage,
			Details: domain.OrphanCountWarningDetails{
				PeriodFromUTC: period.FromUTC,
				PeriodToUTC:   period.ToUTC,
				OrphanCount:   orphanCount,
				Threshold:     domain.DefaultOrphanCountThreshold,
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

		exceededByMonthSpend := stats.MonthSpendMinor > 0 && (stats.OrphanSpendMinor*10000 > int64(domain.DefaultOrphanSpendingThresholdBPS)*stats.MonthSpendMinor)
		exceededByCap := false
		capAmountMinor := int64(0)
		if capItem, ok := caps[key]; ok {
			capAmountMinor = capItem.CapAmountMinor
			if capAmountMinor > 0 {
				exceededByCap = stats.OrphanSpendMinor*10000 > int64(domain.DefaultOrphanSpendingThresholdBPS)*capAmountMinor
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
				ThresholdBPS:   domain.DefaultOrphanSpendingThresholdBPS,
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

type groupCurrencyKey struct {
	PeriodKey    string
	CurrencyCode string
}

type categoryCurrencyKey struct {
	CategoryID   int64
	HasCategory  bool
	CurrencyCode string
}

func toCategoryCurrencyKey(entry domain.Entry) categoryCurrencyKey {
	if entry.CategoryID == nil {
		return categoryCurrencyKey{
			HasCategory:  false,
			CurrencyCode: entry.CurrencyCode,
		}
	}
	return categoryCurrencyKey{
		CategoryID:   *entry.CategoryID,
		HasCategory:  true,
		CurrencyCode: entry.CurrencyCode,
	}
}

func mapCurrencyTotals(values map[string]int64) []domain.CurrencyTotal {
	currencies := make([]string, 0, len(values))
	for currency := range values {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)

	output := make([]domain.CurrencyTotal, 0, len(currencies))
	for _, currency := range currencies {
		output = append(output, domain.CurrencyTotal{
			CurrencyCode: currency,
			TotalMinor:   values[currency],
		})
	}
	return output
}

func mapGroupTotals(values map[groupCurrencyKey]int64) []domain.GroupTotal {
	keys := make([]groupCurrencyKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].PeriodKey != keys[j].PeriodKey {
			return keys[i].PeriodKey < keys[j].PeriodKey
		}
		return keys[i].CurrencyCode < keys[j].CurrencyCode
	})

	output := make([]domain.GroupTotal, 0, len(keys))
	for _, key := range keys {
		output = append(output, domain.GroupTotal{
			PeriodKey:    key.PeriodKey,
			CurrencyCode: key.CurrencyCode,
			TotalMinor:   values[key],
		})
	}
	return output
}

func mapCategoryTotals(values map[categoryCurrencyKey]int64) []domain.CategoryTotal {
	keys := make([]categoryCurrencyKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].HasCategory != keys[j].HasCategory {
			return !keys[i].HasCategory
		}
		if keys[i].CategoryID != keys[j].CategoryID {
			return keys[i].CategoryID < keys[j].CategoryID
		}
		return keys[i].CurrencyCode < keys[j].CurrencyCode
	})

	output := make([]domain.CategoryTotal, 0, len(keys))
	for _, key := range keys {
		item := domain.CategoryTotal{
			CurrencyCode: key.CurrencyCode,
			TotalMinor:   values[key],
		}
		if key.HasCategory {
			categoryID := key.CategoryID
			item.CategoryID = &categoryID
			item.CategoryKey = fmt.Sprintf("category:%d", key.CategoryID)
			item.CategoryLabel = fmt.Sprintf("Category %d", key.CategoryID)
		} else {
			item.CategoryKey = domain.CategoryOrphanKey
			item.CategoryLabel = domain.CategoryOrphanLabel
		}
		output = append(output, item)
	}

	return output
}

func mapNetTotals(earningsByCurrency, spendingByCurrency map[string]int64) []domain.CurrencyTotal {
	netByCurrency := map[string]int64{}
	for currency, total := range earningsByCurrency {
		netByCurrency[currency] += total
	}
	for currency, total := range spendingByCurrency {
		netByCurrency[currency] -= total
	}
	return mapCurrencyTotals(netByCurrency)
}

func sortEntriesDeterministic(entries []domain.Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].TransactionDateUTC != entries[j].TransactionDateUTC {
			return entries[i].TransactionDateUTC < entries[j].TransactionDateUTC
		}
		if entries[i].Type != entries[j].Type {
			return entries[i].Type < entries[j].Type
		}
		if entries[i].CurrencyCode != entries[j].CurrencyCode {
			return entries[i].CurrencyCode < entries[j].CurrencyCode
		}
		if entries[i].AmountMinor != entries[j].AmountMinor {
			return entries[i].AmountMinor < entries[j].AmountMinor
		}
		return entries[i].ID < entries[j].ID
	})
}
