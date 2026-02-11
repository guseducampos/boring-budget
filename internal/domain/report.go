package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	ReportScopeRange     = "range"
	ReportScopeMonthly   = "monthly"
	ReportScopeBimonthly = "bimonthly"
	ReportScopeQuarterly = "quarterly"

	ReportGroupingDay   = "day"
	ReportGroupingWeek  = "week"
	ReportGroupingMonth = "month"

	CategoryOrphanKey   = "orphan"
	CategoryOrphanLabel = "Orphan"
	CategoryUnknownLabel = "Unknown Category"

	DefaultOrphanCountThreshold       = 5
	DefaultOrphanSpendingThresholdBPS = 500

	WarningCodeOrphanCountExceeded    = "ORPHAN_COUNT_THRESHOLD_EXCEEDED"
	WarningCodeOrphanSpendingExceeded = "ORPHAN_SPENDING_THRESHOLD_EXCEEDED"
	OrphanCountWarningMessage         = "Orphan entries exceed the configured threshold for the selected period."
	OrphanSpendingWarningMessage      = "Orphan spending exceeds the configured threshold for one or more months."
)

var (
	ErrInvalidReportScope    = errors.New("invalid report scope")
	ErrInvalidReportGrouping = errors.New("invalid report grouping")
	ErrInvalidReportPeriod   = errors.New("invalid report period")
)

type ReportPeriodInput struct {
	Scope       string
	MonthKey    string
	DateFromUTC string
	DateToUTC   string
}

type ReportPeriod struct {
	Scope    string `json:"scope"`
	MonthKey string `json:"month_key,omitempty"`
	FromUTC  string `json:"from_utc"`
	ToUTC    string `json:"to_utc"`
}

type CurrencyTotal struct {
	CurrencyCode string `json:"currency_code"`
	TotalMinor   int64  `json:"total_minor"`
}

type GroupTotal struct {
	PeriodKey    string `json:"period_key"`
	CurrencyCode string `json:"currency_code"`
	TotalMinor   int64  `json:"total_minor"`
}

type CategoryTotal struct {
	CategoryID    *int64 `json:"category_id,omitempty"`
	CategoryKey   string `json:"category_key"`
	CategoryLabel string `json:"category_label"`
	CurrencyCode  string `json:"currency_code"`
	TotalMinor    int64  `json:"total_minor"`
}

type ReportSection struct {
	ByCurrency []CurrencyTotal `json:"by_currency"`
	Groups     []GroupTotal    `json:"groups"`
	Categories []CategoryTotal `json:"categories"`
}

type ReportNet struct {
	ByCurrency []CurrencyTotal `json:"by_currency"`
}

type ReportCapStatus struct {
	MonthKey        string `json:"month_key"`
	CurrencyCode    string `json:"currency_code"`
	CapAmountMinor  int64  `json:"cap_amount_minor"`
	SpendTotalMinor int64  `json:"spend_total_minor"`
	OverspendMinor  int64  `json:"overspend_minor"`
	IsExceeded      bool   `json:"is_exceeded"`
}

type Report struct {
	Period     ReportPeriod       `json:"period"`
	Grouping   string             `json:"grouping"`
	Earnings   ReportSection      `json:"earnings"`
	Spending   ReportSection      `json:"spending"`
	Net        ReportNet          `json:"net"`
	Converted  *ConvertedSummary  `json:"converted,omitempty"`
	CapStatus  []ReportCapStatus  `json:"cap_status"`
	CapChanges []MonthlyCapChange `json:"cap_changes"`
}

type CurrencyNet struct {
	CurrencyCode string `json:"currency_code"`
	NetMinor     int64  `json:"net_minor"`
}

type BalanceView struct {
	ByCurrency []CurrencyNet `json:"by_currency"`
}

type ConvertedBalanceView struct {
	TargetCurrency   string `json:"target_currency"`
	NetMinor         int64  `json:"net_minor"`
	UsedEstimateRate bool   `json:"used_estimate_rate"`
}

type BalanceViews struct {
	Lifetime          *BalanceView          `json:"lifetime,omitempty"`
	Range             *BalanceView          `json:"range,omitempty"`
	LifetimeConverted *ConvertedBalanceView `json:"lifetime_converted,omitempty"`
	RangeConverted    *ConvertedBalanceView `json:"range_converted,omitempty"`
}

type OrphanCountWarningDetails struct {
	PeriodFromUTC string `json:"period_from_utc"`
	PeriodToUTC   string `json:"period_to_utc"`
	OrphanCount   int    `json:"orphan_count"`
	Threshold     int    `json:"threshold"`
}

type OrphanSpendingWarningDetails struct {
	MonthKey       string   `json:"month_key"`
	CurrencyCode   string   `json:"currency_code"`
	OrphanSpend    int64    `json:"orphan_spend_minor"`
	MonthSpend     int64    `json:"month_spend_minor"`
	CapAmount      *int64   `json:"cap_amount_minor"`
	ThresholdBPS   int      `json:"threshold_bps"`
	TriggeredBy    []string `json:"triggered_by"`
	RatioToSpendBP int64    `json:"ratio_to_month_spend_bps"`
	RatioToCapBP   int64    `json:"ratio_to_cap_bps"`
}

func NormalizeReportScope(scope string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(scope))
	if normalized == "" {
		return ReportScopeRange, nil
	}

	switch normalized {
	case ReportScopeRange, ReportScopeMonthly, ReportScopeBimonthly, ReportScopeQuarterly:
		return normalized, nil
	default:
		return "", ErrInvalidReportScope
	}
}

func NormalizeReportGrouping(grouping string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(grouping))
	if normalized == "" {
		return ReportGroupingMonth, nil
	}

	switch normalized {
	case ReportGroupingDay, ReportGroupingWeek, ReportGroupingMonth:
		return normalized, nil
	default:
		return "", ErrInvalidReportGrouping
	}
}

func BuildReportPeriod(input ReportPeriodInput) (ReportPeriod, error) {
	scope, err := NormalizeReportScope(input.Scope)
	if err != nil {
		return ReportPeriod{}, err
	}

	switch scope {
	case ReportScopeRange:
		fromUTC, toUTC, err := normalizeRangePeriod(input.DateFromUTC, input.DateToUTC)
		if err != nil {
			return ReportPeriod{}, err
		}
		return ReportPeriod{
			Scope:   scope,
			FromUTC: fromUTC,
			ToUTC:   toUTC,
		}, nil
	case ReportScopeMonthly, ReportScopeBimonthly, ReportScopeQuarterly:
		return buildPresetPeriod(scope, input.MonthKey)
	default:
		return ReportPeriod{}, ErrInvalidReportScope
	}
}

func MonthKeysInPeriod(fromUTC, toUTC string) ([]string, error) {
	from, err := parseTimestampUTC(fromUTC)
	if err != nil {
		return nil, ErrInvalidReportPeriod
	}
	to, err := parseTimestampUTC(toUTC)
	if err != nil {
		return nil, ErrInvalidReportPeriod
	}
	if from.After(to) {
		return nil, ErrInvalidReportPeriod
	}

	current := time.Date(from.UTC().Year(), from.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(to.UTC().Year(), to.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)

	months := []string{}
	for !current.After(last) {
		months = append(months, current.Format("2006-01"))
		current = current.AddDate(0, 1, 0)
	}

	return months, nil
}

func PeriodKeyForTransaction(transactionDateUTC string, grouping string) (string, error) {
	normalizedGrouping, err := NormalizeReportGrouping(grouping)
	if err != nil {
		return "", err
	}

	parsedDate, err := parseTimestampUTC(transactionDateUTC)
	if err != nil {
		return "", ErrInvalidTransactionDate
	}

	switch normalizedGrouping {
	case ReportGroupingDay:
		return parsedDate.UTC().Format("2006-01-02"), nil
	case ReportGroupingWeek:
		year, week := parsedDate.UTC().ISOWeek()
		return fmt.Sprintf("%04d-W%02d", year, week), nil
	case ReportGroupingMonth:
		return parsedDate.UTC().Format("2006-01"), nil
	default:
		return "", ErrInvalidReportGrouping
	}
}

func buildPresetPeriod(scope, monthKey string) (ReportPeriod, error) {
	normalizedMonth, err := NormalizeMonthKey(monthKey)
	if err != nil {
		return ReportPeriod{}, err
	}

	start, err := time.Parse("2006-01", normalizedMonth)
	if err != nil {
		return ReportPeriod{}, ErrInvalidReportPeriod
	}

	months := 1
	switch scope {
	case ReportScopeMonthly:
		months = 1
	case ReportScopeBimonthly:
		months = 2
	case ReportScopeQuarterly:
		months = 3
	default:
		return ReportPeriod{}, ErrInvalidReportScope
	}

	startUTC := start.UTC()
	endUTC := startUTC.AddDate(0, months, 0).Add(-time.Nanosecond)

	return ReportPeriod{
		Scope:    scope,
		MonthKey: normalizedMonth,
		FromUTC:  startUTC.Format(time.RFC3339Nano),
		ToUTC:    endUTC.Format(time.RFC3339Nano),
	}, nil
}

func normalizeRangePeriod(fromRaw, toRaw string) (string, string, error) {
	fromUTC, err := parsePeriodBoundary(fromRaw, false)
	if err != nil {
		return "", "", ErrInvalidReportPeriod
	}

	toUTC, err := parsePeriodBoundary(toRaw, true)
	if err != nil {
		return "", "", ErrInvalidReportPeriod
	}

	if fromUTC.After(toUTC) {
		return "", "", ErrInvalidReportPeriod
	}

	return fromUTC.Format(time.RFC3339Nano), toUTC.Format(time.RFC3339Nano), nil
}

func parsePeriodBoundary(value string, inclusiveEndOfDay bool) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, ErrInvalidReportPeriod
	}

	if parsed, err := parseTimestampUTC(trimmed); err == nil {
		return parsed, nil
	}

	dateOnly, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return time.Time{}, ErrInvalidReportPeriod
	}

	if inclusiveEndOfDay {
		return dateOnly.UTC().AddDate(0, 0, 1).Add(-time.Nanosecond), nil
	}
	return dateOnly.UTC(), nil
}

func parseTimestampUTC(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, strings.TrimSpace(value))
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, ErrInvalidReportPeriod
}
