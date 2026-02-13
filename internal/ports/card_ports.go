package ports

import (
	"context"
	"database/sql"
	"errors"
)

const (
	CardTypeCredit = "credit"
	CardTypeDebit  = "debit"

	PaymentMethodCash = "cash"
	PaymentMethodCard = "card"

	LiabilityEventCharge     = "charge"
	LiabilityEventPayment    = "payment"
	LiabilityEventAdjustment = "adjustment"

	LiabilityStateOwes    = "owes"
	LiabilityStateSettled = "settled"
	LiabilityStateInFavor = "in_favor"
)

var (
	ErrCardInvalidID                    = errors.New("invalid card id")
	ErrCardNicknameRequired             = errors.New("card nickname is required")
	ErrCardNicknameConflict             = errors.New("card nickname conflict")
	ErrCardNotFound                     = errors.New("card not found")
	ErrCardInvalidLast4                 = errors.New("invalid card last4")
	ErrCardBrandRequired                = errors.New("card brand is required")
	ErrCardInvalidType                  = errors.New("invalid card type")
	ErrCardInvalidDueDay                = errors.New("invalid due day")
	ErrCardDueDayRequired               = errors.New("due day required for credit card")
	ErrCardDueDayNotAllowed             = errors.New("due day is not allowed for debit card")
	ErrCardInvalidAsOfDate              = errors.New("invalid as_of date")
	ErrCardLookupTextRequired           = errors.New("lookup text is required")
	ErrTransactionInvalidID             = errors.New("invalid transaction id")
	ErrTransactionNotFound              = errors.New("transaction not found")
	ErrTransactionPaymentMethodNotFound = errors.New("transaction payment method not found")
	ErrPaymentMethodInvalid             = errors.New("invalid payment method")
	ErrPaymentMethodCardRequired        = errors.New("card id is required for card payment method")
	ErrPaymentMethodCardNotAllowed      = errors.New("card id is not allowed for cash payment method")
	ErrCurrencyCodeInvalid              = errors.New("invalid currency code")
	ErrLiabilityEventTypeInvalid        = errors.New("invalid liability event type")
	ErrLiabilityAmountInvalid           = errors.New("invalid liability amount")
)

type Card struct {
	ID           int64   `json:"id"`
	Nickname     string  `json:"nickname"`
	Description  *string `json:"description,omitempty"`
	Last4        string  `json:"last4"`
	Brand        string  `json:"brand"`
	CardType     string  `json:"card_type"`
	DueDay       *int64  `json:"due_day,omitempty"`
	CreatedAtUTC string  `json:"created_at_utc"`
	UpdatedAtUTC string  `json:"updated_at_utc"`
	DeletedAtUTC *string `json:"deleted_at_utc,omitempty"`
}

type CardCreateInput struct {
	Nickname    string
	Description *string
	Last4       string
	Brand       string
	CardType    string
	DueDay      *int64
}

type CardUpdateInput struct {
	ID             int64
	Nickname       *string
	SetDescription bool
	Description    *string
	Last4          *string
	Brand          *string
	CardType       *string
	SetDueDay      bool
	DueDay         *int64
}

type CardListFilter struct {
	CardType       string
	IncludeDeleted bool
}

type CardDeleteResult struct {
	CardID       int64  `json:"card_id"`
	DeletedAtUTC string `json:"deleted_at_utc"`
}

type CardDue struct {
	CardID      int64  `json:"card_id"`
	Nickname    string `json:"nickname"`
	DueDay      int64  `json:"due_day"`
	AsOfDate    string `json:"as_of_date"`
	NextDueDate string `json:"next_due_date"`
}

type TransactionPaymentMethod struct {
	TransactionID int64  `json:"transaction_id"`
	MethodType    string `json:"method_type"`
	CardID        *int64 `json:"card_id,omitempty"`
	CreatedAtUTC  string `json:"created_at_utc"`
	UpdatedAtUTC  string `json:"updated_at_utc"`
}

type TransactionPaymentMethodUpsertInput struct {
	TransactionID int64
	MethodType    string
	CardID        *int64
}

type CreditLiabilityEvent struct {
	ID                     int64   `json:"id"`
	CardID                 int64   `json:"card_id"`
	CurrencyCode           string  `json:"currency_code"`
	EventType              string  `json:"event_type"`
	AmountMinorSigned      int64   `json:"amount_minor_signed"`
	ReferenceTransactionID *int64  `json:"reference_transaction_id,omitempty"`
	Note                   *string `json:"note,omitempty"`
	CreatedAtUTC           string  `json:"created_at_utc"`
}

type CreditLiabilityEventInput struct {
	CardID                 int64
	CurrencyCode           string
	EventType              string
	AmountMinorSigned      int64
	ReferenceTransactionID *int64
	Note                   *string
}

type CardPaymentEventInput struct {
	CardID                 int64
	CurrencyCode           string
	AmountMinor            int64
	ReferenceTransactionID *int64
	Note                   *string
}

type CardDebtBucket struct {
	CardID         int64  `json:"card_id"`
	CurrencyCode   string `json:"currency_code"`
	BalanceMinor   int64  `json:"balance_minor"`
	State          string `json:"state"`
	LastEventAtUTC string `json:"last_event_at_utc"`
}

type CardRepository interface {
	AddCard(ctx context.Context, input CardCreateInput) (Card, error)
	GetCardByID(ctx context.Context, id int64, includeDeleted bool) (Card, error)
	ListCards(ctx context.Context, filter CardListFilter) ([]Card, error)
	SearchCards(ctx context.Context, lookup string, limit int32) ([]Card, error)
	UpdateCard(ctx context.Context, input CardUpdateInput) (Card, error)
	DeleteCard(ctx context.Context, id int64) (CardDeleteResult, error)
	GetCardDue(ctx context.Context, cardID int64, asOfDate string) (CardDue, error)
	ListCardDues(ctx context.Context, asOfDate string) ([]CardDue, error)
	UpsertTransactionPaymentMethod(ctx context.Context, input TransactionPaymentMethodUpsertInput) (TransactionPaymentMethod, error)
	GetTransactionPaymentMethod(ctx context.Context, transactionID int64) (TransactionPaymentMethod, error)
	AddLiabilityEvent(ctx context.Context, input CreditLiabilityEventInput) (CreditLiabilityEvent, error)
	AddPaymentEvent(ctx context.Context, input CardPaymentEventInput) (CreditLiabilityEvent, error)
	ListLiabilityEvents(ctx context.Context, cardID int64, currencyCode string) ([]CreditLiabilityEvent, error)
	GetDebtSummaryByCard(ctx context.Context, cardID int64) ([]CardDebtBucket, error)
	GetDebtSummary(ctx context.Context) ([]CardDebtBucket, error)
	GetDebtBalance(ctx context.Context, cardID int64, currencyCode string) (int64, error)
}

type CardRepositoryTxBinder interface {
	BindTx(tx *sql.Tx) CardRepository
}
