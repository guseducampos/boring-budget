package domain

import (
	"errors"
	"strings"
)

const (
	SavingsEventTypeTransferToSavings = "transfer_to_savings"
	SavingsEventTypeIndependentAdd    = "independent_add"
)

var (
	ErrInvalidSavingsEventType = errors.New("invalid savings event type")
)

type SavingsEvent struct {
	ID                       int64  `json:"id"`
	EventType                string `json:"event_type"`
	AmountMinor              int64  `json:"amount_minor"`
	CurrencyCode             string `json:"currency_code"`
	EventDateUTC             string `json:"event_date_utc"`
	SourceBankAccountID      *int64 `json:"source_bank_account_id,omitempty"`
	DestinationBankAccountID *int64 `json:"destination_bank_account_id,omitempty"`
	Note                     string `json:"note,omitempty"`
	CreatedAtUTC             string `json:"created_at_utc"`
}

type SavingsEventAddInput struct {
	EventType                string
	AmountMinor              int64
	CurrencyCode             string
	EventDateUTC             string
	SourceBankAccountID      *int64
	DestinationBankAccountID *int64
	Note                     string
}

type SavingsEventListFilter struct {
	DateFromUTC string
	DateToUTC   string
	EventType   string
}

type SavingsCurrencyBalance struct {
	CurrencyCode        string `json:"currency_code"`
	GeneralBalanceMinor int64  `json:"general_balance_minor"`
	SavingsBalanceMinor int64  `json:"savings_balance_minor"`
	TotalBalanceMinor   int64  `json:"total_balance_minor"`
}

type SavingsBalanceView struct {
	ByCurrency []SavingsCurrencyBalance `json:"by_currency"`
}

type SavingsBalanceViews struct {
	Lifetime *SavingsBalanceView `json:"lifetime,omitempty"`
	Range    *SavingsBalanceView `json:"range,omitempty"`
}

func NormalizeSavingsEventType(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case SavingsEventTypeTransferToSavings, SavingsEventTypeIndependentAdd:
		return normalized, nil
	default:
		return "", ErrInvalidSavingsEventType
	}
}

func NormalizeOptionalSavingsEventType(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	return NormalizeSavingsEventType(trimmed)
}
