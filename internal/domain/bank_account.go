package domain

import (
	"errors"
	"strings"
)

var (
	ErrInvalidBankAccountID      = errors.New("invalid bank account id")
	ErrBankAccountAliasRequired  = errors.New("bank account alias is required")
	ErrBankAccountAliasConflict  = errors.New("bank account alias conflict")
	ErrBankAccountLast4Invalid   = errors.New("bank account last4 is invalid")
	ErrBankAccountNotFound       = errors.New("bank account not found")
	ErrNoBankAccountUpdateFields = errors.New("no bank account update fields")
	ErrInvalidBankAccountLookup  = errors.New("invalid bank account lookup text")
	ErrInvalidBalanceLinkTarget  = errors.New("invalid balance link target")
)

const (
	BalanceLinkTargetGeneral = "general_balance"
	BalanceLinkTargetSavings = "savings"
)

type BankAccount struct {
	ID           int64   `json:"id"`
	Alias        string  `json:"alias"`
	Last4        string  `json:"last4"`
	CreatedAtUTC string  `json:"created_at_utc"`
	UpdatedAtUTC string  `json:"updated_at_utc"`
	DeletedAtUTC *string `json:"deleted_at_utc,omitempty"`
}

type BankAccountDeleteResult struct {
	BankAccountID int64  `json:"bank_account_id"`
	DeletedAtUTC  string `json:"deleted_at_utc"`
}

type BankAccountAddInput struct {
	Alias string
	Last4 string
}

type BankAccountListFilter struct {
	Lookup         string
	IncludeDeleted bool
}

type BankAccountUpdateInput struct {
	ID    int64
	Alias *string
	Last4 *string
}

type BalanceAccountLink struct {
	Target      string       `json:"target"`
	BankAccount *BankAccount `json:"bank_account,omitempty"`
}

func ValidateBankAccountID(id int64) error {
	if id <= 0 {
		return ErrInvalidBankAccountID
	}
	return nil
}

func NormalizeBankAccountAlias(alias string) (string, error) {
	normalized := strings.TrimSpace(alias)
	if normalized == "" {
		return "", ErrBankAccountAliasRequired
	}
	return normalized, nil
}

func NormalizeBankAccountLast4(last4 string) (string, error) {
	normalized := strings.TrimSpace(last4)
	if len(normalized) != 4 {
		return "", ErrBankAccountLast4Invalid
	}
	for i := 0; i < len(normalized); i++ {
		if normalized[i] < '0' || normalized[i] > '9' {
			return "", ErrBankAccountLast4Invalid
		}
	}
	return normalized, nil
}

func NormalizeBankAccountLookup(lookup string) (string, error) {
	normalized := strings.TrimSpace(lookup)
	if normalized == "" {
		return "", ErrInvalidBankAccountLookup
	}
	return normalized, nil
}

func NormalizeBalanceLinkTarget(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case BalanceLinkTargetGeneral, BalanceLinkTargetSavings:
		return normalized, nil
	default:
		return "", ErrInvalidBalanceLinkTarget
	}
}

func NormalizeBankAccountAddInput(input BankAccountAddInput) (BankAccountAddInput, error) {
	alias, err := NormalizeBankAccountAlias(input.Alias)
	if err != nil {
		return BankAccountAddInput{}, err
	}

	last4, err := NormalizeBankAccountLast4(input.Last4)
	if err != nil {
		return BankAccountAddInput{}, err
	}

	return BankAccountAddInput{
		Alias: alias,
		Last4: last4,
	}, nil
}
