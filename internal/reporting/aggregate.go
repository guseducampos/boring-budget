package reporting

import (
	"fmt"
	"sort"
	"strings"

	"boring-budget/internal/domain"
)

type AggregateResult struct {
	Earnings domain.ReportSection
	Spending domain.ReportSection
	Net      domain.ReportNet
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

type CategoryLabelResolver func(categoryID int64) string

func BuildAggregate(entries []domain.Entry, grouping string, categoryLabelResolver CategoryLabelResolver) (AggregateResult, error) {
	earnByCurrency := map[string]int64{}
	spendByCurrency := map[string]int64{}
	earnGroups := map[groupCurrencyKey]int64{}
	spendGroups := map[groupCurrencyKey]int64{}
	earnCategories := map[categoryCurrencyKey]int64{}
	spendCategories := map[categoryCurrencyKey]int64{}

	for _, entry := range entries {
		periodKey, err := domain.PeriodKeyForTransaction(entry.TransactionDateUTC, grouping)
		if err != nil {
			return AggregateResult{}, err
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

	return AggregateResult{
		Earnings: domain.ReportSection{
			ByCurrency: mapCurrencyTotals(earnByCurrency),
			Groups:     mapGroupTotals(earnGroups),
			Categories: mapCategoryTotals(earnCategories, categoryLabelResolver),
		},
		Spending: domain.ReportSection{
			ByCurrency: mapCurrencyTotals(spendByCurrency),
			Groups:     mapGroupTotals(spendGroups),
			Categories: mapCategoryTotals(spendCategories, categoryLabelResolver),
		},
		Net: domain.ReportNet{
			ByCurrency: mapNetTotals(earnByCurrency, spendByCurrency),
		},
	}, nil
}

func SortEntriesDeterministic(entries []domain.Entry) {
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

func toCategoryCurrencyKey(entry domain.Entry) categoryCurrencyKey {
	if entry.CategoryID == nil {
		return categoryCurrencyKey{HasCategory: false, CurrencyCode: entry.CurrencyCode}
	}
	return categoryCurrencyKey{CategoryID: *entry.CategoryID, HasCategory: true, CurrencyCode: entry.CurrencyCode}
}

func mapCurrencyTotals(values map[string]int64) []domain.CurrencyTotal {
	currencies := make([]string, 0, len(values))
	for currency := range values {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)

	output := make([]domain.CurrencyTotal, 0, len(currencies))
	for _, currency := range currencies {
		output = append(output, domain.CurrencyTotal{CurrencyCode: currency, TotalMinor: values[currency]})
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
		output = append(output, domain.GroupTotal{PeriodKey: key.PeriodKey, CurrencyCode: key.CurrencyCode, TotalMinor: values[key]})
	}
	return output
}

func mapCategoryTotals(values map[categoryCurrencyKey]int64, categoryLabelResolver CategoryLabelResolver) []domain.CategoryTotal {
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
		item := domain.CategoryTotal{CurrencyCode: key.CurrencyCode, TotalMinor: values[key]}
		if key.HasCategory {
			categoryID := key.CategoryID
			item.CategoryID = &categoryID
			item.CategoryKey = fmt.Sprintf("category:%d", key.CategoryID)
			label := ""
			if categoryLabelResolver != nil {
				label = strings.TrimSpace(categoryLabelResolver(key.CategoryID))
			}
			if label == "" {
				label = domain.CategoryUnknownLabel
			}
			item.CategoryLabel = label
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
