package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	WarningCodeCapExceeded    = "CAP_EXCEEDED"
	CapExceededWarningMessage = "Expense saved, monthly cap exceeded."
)

var (
	ErrInvalidMonthKey      = errors.New("invalid month key")
	ErrInvalidCapAmount     = errors.New("invalid cap amount_minor")
	ErrCapNotFound          = errors.New("cap not found")
	ErrInvalidMonthDateTime = errors.New("invalid month datetime")
)

type MonthlyCap struct {
	ID           int64  `json:"id"`
	MonthKey     string `json:"month_key"`
	AmountMinor  int64  `json:"amount_minor"`
	CurrencyCode string `json:"currency_code"`
	CreatedAtUTC string `json:"created_at_utc"`
	UpdatedAtUTC string `json:"updated_at_utc"`
}

type MonthlyCapChange struct {
	ID             int64  `json:"id"`
	MonthKey       string `json:"month_key"`
	OldAmountMinor *int64 `json:"old_amount_minor"`
	NewAmountMinor int64  `json:"new_amount_minor"`
	CurrencyCode   string `json:"currency_code"`
	ChangedAtUTC   string `json:"changed_at_utc"`
}

type CapSetInput struct {
	MonthKey     string
	AmountMinor  int64
	CurrencyCode string
}

type MoneyAmount struct {
	AmountMinor  int64  `json:"amount_minor"`
	CurrencyCode string `json:"currency_code"`
}

type CapExceededWarningDetails struct {
	MonthKey        string      `json:"month_key"`
	CapAmount       MoneyAmount `json:"cap_amount"`
	NewSpendTotal   MoneyAmount `json:"new_spend_total"`
	OverspendAmount MoneyAmount `json:"overspend_amount"`
}

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func NormalizeMonthKey(monthKey string) (string, error) {
	normalized := strings.TrimSpace(monthKey)
	if normalized == "" {
		return "", ErrInvalidMonthKey
	}

	parsed, err := time.Parse("2006-01", normalized)
	if err != nil {
		return "", ErrInvalidMonthKey
	}
	return parsed.UTC().Format("2006-01"), nil
}

func ValidateCapAmountMinor(amountMinor int64) error {
	if amountMinor <= 0 {
		return ErrInvalidCapAmount
	}
	return nil
}

func NormalizeCapSetInput(input CapSetInput) (CapSetInput, error) {
	monthKey, err := NormalizeMonthKey(input.MonthKey)
	if err != nil {
		return CapSetInput{}, err
	}

	if err := ValidateCapAmountMinor(input.AmountMinor); err != nil {
		return CapSetInput{}, err
	}

	currencyCode, err := NormalizeCurrencyCode(input.CurrencyCode)
	if err != nil {
		return CapSetInput{}, err
	}

	return CapSetInput{
		MonthKey:     monthKey,
		AmountMinor:  input.AmountMinor,
		CurrencyCode: currencyCode,
	}, nil
}

func MonthKeyFromDateTimeUTC(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", ErrInvalidMonthDateTime
	}

	parsed, err := time.Parse(time.RFC3339Nano, normalized)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, normalized)
		if err != nil {
			return "", ErrInvalidMonthDateTime
		}
	}

	return parsed.UTC().Format("2006-01"), nil
}

func MonthRangeUTC(monthKey string) (string, string, error) {
	normalized, err := NormalizeMonthKey(monthKey)
	if err != nil {
		return "", "", err
	}

	start, err := time.Parse("2006-01", normalized)
	if err != nil {
		return "", "", fmt.Errorf("month range parse start: %w", err)
	}
	startUTC := start.UTC()
	endUTC := startUTC.AddDate(0, 1, 0)

	return startUTC.Format(time.RFC3339Nano), endUTC.Format(time.RFC3339Nano), nil
}
