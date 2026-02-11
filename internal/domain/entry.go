package domain

import (
	"errors"
	"sort"
	"strings"
	"time"
)

const (
	EntryTypeIncome  = "income"
	EntryTypeExpense = "expense"

	LabelFilterModeAny  = "ANY"
	LabelFilterModeAll  = "ALL"
	LabelFilterModeNone = "NONE"
)

var (
	ErrInvalidEntryType       = errors.New("invalid entry type")
	ErrInvalidAmountMinor     = errors.New("invalid amount_minor")
	ErrInvalidCurrencyCode    = errors.New("invalid currency code")
	ErrInvalidTransactionDate = errors.New("invalid transaction date")
	ErrInvalidEntryID         = errors.New("invalid entry id")
	ErrNoEntryUpdateFields    = errors.New("no entry update fields")
	ErrInvalidDateRange       = errors.New("invalid date range")
	ErrInvalidLabelMode       = errors.New("invalid label filter mode")
	ErrEntryNotFound          = errors.New("entry not found")
)

type Entry struct {
	ID                 int64   `json:"id"`
	Type               string  `json:"type"`
	AmountMinor        int64   `json:"amount_minor"`
	CurrencyCode       string  `json:"currency_code"`
	TransactionDateUTC string  `json:"transaction_date_utc"`
	CategoryID         *int64  `json:"category_id,omitempty"`
	LabelIDs           []int64 `json:"label_ids,omitempty"`
	Note               string  `json:"note,omitempty"`
	CreatedAtUTC       string  `json:"created_at_utc"`
	UpdatedAtUTC       string  `json:"updated_at_utc"`
}

type EntryAddInput struct {
	Type               string
	AmountMinor        int64
	CurrencyCode       string
	TransactionDateUTC string
	CategoryID         *int64
	LabelIDs           []int64
	Note               string
}

type EntryUpdateInput struct {
	ID                 int64
	Type               *string
	AmountMinor        *int64
	CurrencyCode       *string
	TransactionDateUTC *string
	SetCategory        bool
	CategoryID         *int64
	SetLabelIDs        bool
	LabelIDs           []int64
	SetNote            bool
	Note               *string
}

type EntryListFilter struct {
	Type         string
	CategoryID   *int64
	DateFromUTC  string
	DateToUTC    string
	NoteContains string
	LabelIDs     []int64
	LabelMode    string
}

type EntryDeleteResult struct {
	EntryID        int64  `json:"entry_id"`
	DeletedAtUTC   string `json:"deleted_at_utc"`
	DetachedLabels int64  `json:"detached_labels"`
}

func ValidateEntryID(id int64) error {
	if id <= 0 {
		return ErrInvalidEntryID
	}
	return nil
}

func HasEntryUpdateChanges(input EntryUpdateInput) bool {
	return input.Type != nil ||
		input.AmountMinor != nil ||
		input.CurrencyCode != nil ||
		input.TransactionDateUTC != nil ||
		input.SetCategory ||
		input.SetLabelIDs ||
		input.SetNote
}

func NormalizeEntryType(entryType string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(entryType))
	if normalized != EntryTypeIncome && normalized != EntryTypeExpense {
		return "", ErrInvalidEntryType
	}
	return normalized, nil
}

func ValidateAmountMinor(amountMinor int64) error {
	if amountMinor <= 0 {
		return ErrInvalidAmountMinor
	}
	return nil
}

func NormalizeCurrencyCode(currencyCode string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(currencyCode))
	if len(normalized) != 3 {
		return "", ErrInvalidCurrencyCode
	}
	for i := 0; i < len(normalized); i++ {
		if normalized[i] < 'A' || normalized[i] > 'Z' {
			return "", ErrInvalidCurrencyCode
		}
	}
	return normalized, nil
}

func NormalizeTransactionDateUTC(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", ErrInvalidTransactionDate
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		parsed, err := time.Parse(layout, normalized)
		if err == nil {
			return parsed.UTC().Format(time.RFC3339Nano), nil
		}
	}

	return "", ErrInvalidTransactionDate
}

func ValidateOptionalCategoryID(categoryID *int64) error {
	if categoryID == nil {
		return nil
	}
	if *categoryID <= 0 {
		return ErrInvalidCategoryID
	}
	return nil
}

func NormalizeLabelIDs(labelIDs []int64) ([]int64, error) {
	if len(labelIDs) == 0 {
		return nil, nil
	}

	unique := make(map[int64]struct{}, len(labelIDs))
	for _, labelID := range labelIDs {
		if err := ValidateLabelID(labelID); err != nil {
			return nil, err
		}
		unique[labelID] = struct{}{}
	}

	normalized := make([]int64, 0, len(unique))
	for labelID := range unique {
		normalized = append(normalized, labelID)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i] < normalized[j]
	})

	return normalized, nil
}

func NormalizeLabelMode(mode string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(mode))
	if normalized == "" {
		return LabelFilterModeAny, nil
	}

	switch normalized {
	case LabelFilterModeAny, LabelFilterModeAll, LabelFilterModeNone:
		return normalized, nil
	default:
		return "", ErrInvalidLabelMode
	}
}

func NormalizeOptionalTransactionDateUTC(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	return NormalizeTransactionDateUTC(trimmed)
}

func ValidateDateRange(fromUTC, toUTC string) error {
	if fromUTC == "" || toUTC == "" {
		return nil
	}

	from, err := time.Parse(time.RFC3339Nano, fromUTC)
	if err != nil {
		return ErrInvalidDateRange
	}
	to, err := time.Parse(time.RFC3339Nano, toUTC)
	if err != nil {
		return ErrInvalidDateRange
	}
	if from.After(to) {
		return ErrInvalidDateRange
	}
	return nil
}
