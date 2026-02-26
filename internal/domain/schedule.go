package domain

import (
	"errors"
	"strings"
	"time"
)

const ScheduleNameMaxLength = 120

var (
	ErrInvalidScheduleID           = errors.New("invalid schedule id")
	ErrScheduleNameRequired        = errors.New("schedule name is required")
	ErrScheduleNameTooLong         = errors.New("schedule name exceeds maximum length")
	ErrInvalidScheduleDayOfMonth   = errors.New("invalid schedule day_of_month")
	ErrScheduleEndMonthBeforeStart = errors.New("schedule end_month is before start_month")
	ErrScheduleNotFound            = errors.New("schedule not found")
	ErrScheduleExecutionNotFound   = errors.New("schedule execution not found")
	ErrInvalidScheduleThroughDate  = errors.New("invalid schedule through-date")
)

type ScheduledPayment struct {
	ID                   int64   `json:"id"`
	Name                 string  `json:"name"`
	AmountMinor          int64   `json:"amount_minor"`
	CurrencyCode         string  `json:"currency_code"`
	DayOfMonth           int     `json:"day_of_month"`
	StartMonthKey        string  `json:"start_month_key"`
	EndMonthKey          *string `json:"end_month_key,omitempty"`
	CategoryID           *int64  `json:"category_id,omitempty"`
	Note                 string  `json:"note,omitempty"`
	CreatedAtUTC         string  `json:"created_at_utc"`
	UpdatedAtUTC         string  `json:"updated_at_utc"`
	DeletedAtUTC         *string `json:"deleted_at_utc,omitempty"`
	LastExecutedMonthKey *string `json:"last_executed_month_key,omitempty"`
}

type ScheduledPaymentAddInput struct {
	Name          string
	AmountMinor   int64
	CurrencyCode  string
	DayOfMonth    int
	StartMonthKey string
	EndMonthKey   *string
	CategoryID    *int64
	Note          string
}

type ScheduledPaymentDeleteResult struct {
	ScheduleID   int64  `json:"schedule_id"`
	DeletedAtUTC string `json:"deleted_at_utc"`
}

type ScheduledPaymentRunInput struct {
	ThroughDateUTC string
	ScheduleID     *int64
	DryRun         bool
}

type ScheduledPaymentRunResult struct {
	ThroughDateUTC string `json:"through_date_utc"`
	ScheduleID     *int64 `json:"schedule_id,omitempty"`
	DryRun         bool   `json:"dry_run"`
	CreatedCount   int    `json:"created_count"`
	SkippedCount   int    `json:"skipped_count"`
}

type ScheduledPaymentExecution struct {
	ID           int64  `json:"id"`
	ScheduleID   int64  `json:"schedule_id"`
	MonthKey     string `json:"month_key"`
	EntryID      *int64 `json:"entry_id,omitempty"`
	CreatedAtUTC string `json:"created_at_utc"`
}

func ValidateScheduleID(id int64) error {
	if id <= 0 {
		return ErrInvalidScheduleID
	}
	return nil
}

func NormalizeScheduledPaymentName(name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", ErrScheduleNameRequired
	}
	if len(normalized) > ScheduleNameMaxLength {
		return "", ErrScheduleNameTooLong
	}
	return normalized, nil
}

func NormalizeScheduleDayOfMonth(day int) (int, error) {
	if day < 1 || day > 28 {
		return 0, ErrInvalidScheduleDayOfMonth
	}
	return day, nil
}

func NormalizeScheduledPaymentAddInput(input ScheduledPaymentAddInput) (ScheduledPaymentAddInput, error) {
	name, err := NormalizeScheduledPaymentName(input.Name)
	if err != nil {
		return ScheduledPaymentAddInput{}, err
	}
	if err := ValidateAmountMinor(input.AmountMinor); err != nil {
		return ScheduledPaymentAddInput{}, err
	}
	currencyCode, err := NormalizeCurrencyCode(input.CurrencyCode)
	if err != nil {
		return ScheduledPaymentAddInput{}, err
	}
	dayOfMonth, err := NormalizeScheduleDayOfMonth(input.DayOfMonth)
	if err != nil {
		return ScheduledPaymentAddInput{}, err
	}
	startMonth, err := NormalizeMonthKey(input.StartMonthKey)
	if err != nil {
		return ScheduledPaymentAddInput{}, err
	}

	var endMonth *string
	if input.EndMonthKey != nil {
		normalizedEndMonth, err := NormalizeMonthKey(*input.EndMonthKey)
		if err != nil {
			return ScheduledPaymentAddInput{}, err
		}
		if normalizedEndMonth < startMonth {
			return ScheduledPaymentAddInput{}, ErrScheduleEndMonthBeforeStart
		}
		endMonth = &normalizedEndMonth
	}

	if err := ValidateOptionalCategoryID(input.CategoryID); err != nil {
		return ScheduledPaymentAddInput{}, err
	}

	var categoryID *int64
	if input.CategoryID != nil {
		value := *input.CategoryID
		categoryID = &value
	}

	return ScheduledPaymentAddInput{
		Name:          name,
		AmountMinor:   input.AmountMinor,
		CurrencyCode:  currencyCode,
		DayOfMonth:    dayOfMonth,
		StartMonthKey: startMonth,
		EndMonthKey:   endMonth,
		CategoryID:    categoryID,
		Note:          strings.TrimSpace(input.Note),
	}, nil
}

func NormalizeScheduledPaymentRunInput(input ScheduledPaymentRunInput) (ScheduledPaymentRunInput, time.Time, error) {
	throughDateRaw := strings.TrimSpace(input.ThroughDateUTC)
	if throughDateRaw == "" {
		return ScheduledPaymentRunInput{}, time.Time{}, ErrInvalidScheduleThroughDate
	}

	throughDateUTC, err := NormalizeTransactionDateUTC(throughDateRaw)
	if err != nil {
		return ScheduledPaymentRunInput{}, time.Time{}, ErrInvalidScheduleThroughDate
	}

	throughTime, err := time.Parse(time.RFC3339Nano, throughDateUTC)
	if err != nil {
		return ScheduledPaymentRunInput{}, time.Time{}, ErrInvalidScheduleThroughDate
	}

	var scheduleID *int64
	if input.ScheduleID != nil {
		if err := ValidateScheduleID(*input.ScheduleID); err != nil {
			return ScheduledPaymentRunInput{}, time.Time{}, err
		}
		value := *input.ScheduleID
		scheduleID = &value
	}

	return ScheduledPaymentRunInput{
		ThroughDateUTC: throughDateUTC,
		ScheduleID:     scheduleID,
		DryRun:         input.DryRun,
	}, throughTime.UTC(), nil
}

func ScheduledPaymentOccurrenceMonths(schedule ScheduledPayment, throughDateUTC time.Time) ([]string, error) {
	startMonthKey, err := NormalizeMonthKey(schedule.StartMonthKey)
	if err != nil {
		return nil, err
	}

	dayOfMonth, err := NormalizeScheduleDayOfMonth(schedule.DayOfMonth)
	if err != nil {
		return nil, err
	}

	throughDateUTC = throughDateUTC.UTC()
	lastMonthStart := time.Date(throughDateUTC.Year(), throughDateUTC.Month(), 1, 0, 0, 0, 0, time.UTC)
	if throughDateUTC.Day() < dayOfMonth {
		lastMonthStart = lastMonthStart.AddDate(0, -1, 0)
	}
	lastMonthKey := lastMonthStart.Format("2006-01")

	effectiveEndMonthKey := lastMonthKey
	if schedule.EndMonthKey != nil {
		normalizedEndMonthKey, err := NormalizeMonthKey(*schedule.EndMonthKey)
		if err != nil {
			return nil, err
		}
		if normalizedEndMonthKey < effectiveEndMonthKey {
			effectiveEndMonthKey = normalizedEndMonthKey
		}
	}

	if effectiveEndMonthKey < startMonthKey {
		return []string{}, nil
	}

	startMonthTime, err := time.Parse("2006-01", startMonthKey)
	if err != nil {
		return nil, ErrInvalidMonthKey
	}
	endMonthTime, err := time.Parse("2006-01", effectiveEndMonthKey)
	if err != nil {
		return nil, ErrInvalidMonthKey
	}

	months := make([]string, 0)
	for current := startMonthTime.UTC(); !current.After(endMonthTime.UTC()); current = current.AddDate(0, 1, 0) {
		months = append(months, current.Format("2006-01"))
	}

	return months, nil
}

func ScheduledPaymentOccurrenceDateUTC(monthKey string, dayOfMonth int) (string, error) {
	normalizedMonthKey, err := NormalizeMonthKey(monthKey)
	if err != nil {
		return "", err
	}

	normalizedDay, err := NormalizeScheduleDayOfMonth(dayOfMonth)
	if err != nil {
		return "", err
	}

	monthStart, err := time.Parse("2006-01", normalizedMonthKey)
	if err != nil {
		return "", ErrInvalidMonthKey
	}

	occurrenceDate := time.Date(monthStart.Year(), monthStart.Month(), normalizedDay, 0, 0, 0, 0, time.UTC)
	return occurrenceDate.UTC().Format(time.RFC3339Nano), nil
}
