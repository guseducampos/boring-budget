package domain

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	CardTypeCredit = "credit"
	CardTypeDebit  = "debit"

	CardBrandVisa       = "VISA"
	CardBrandMastercard = "MASTERCARD"
	CardBrandDiners     = "DINERS"
	CardBrandAmex       = "AMEX"
	CardBrandElo        = "ELO"
	CardBrandDiscover   = "DISCOVER"
	CardBrandOther      = "OTHER"

	CardDebtStateOwes    = "owes"
	CardDebtStateSettled = "settled"
	CardDebtStateInFavor = "in_favor"

	CardLiabilityEventCharge     = "charge"
	CardLiabilityEventPayment    = "payment"
	CardLiabilityEventAdjustment = "adjustment"
)

var (
	ErrInvalidCardID               = errors.New("invalid card id")
	ErrCardNicknameRequired        = errors.New("card nickname is required")
	ErrCardNicknameConflict        = errors.New("card nickname conflict")
	ErrCardLast4Invalid            = errors.New("card last4 is invalid")
	ErrCardBrandRequired           = errors.New("card brand is required")
	ErrInvalidCardBrand            = errors.New("invalid card brand")
	ErrInvalidCardType             = errors.New("invalid card type")
	ErrInvalidCardDueDay           = errors.New("invalid card due day")
	ErrCardDueDayRequiredForCredit = errors.New("card due day is required for credit cards")
	ErrCardDueDayOnlyForCredit     = errors.New("card due day is only valid for credit cards")
	ErrCardNotFound                = errors.New("card not found")
	ErrNoCardUpdateFields          = errors.New("no card update fields")
	ErrCardLookupRequired          = errors.New("card lookup is required")
	ErrCardLookupSelectorConflict  = errors.New("card lookup selector conflict")
	ErrCardLookupAmbiguous         = errors.New("card lookup is ambiguous")
	ErrInvalidCardLookupText       = errors.New("invalid card lookup text")
	ErrInvalidCardAsOfDate         = errors.New("invalid card as_of date")
	ErrCardPaymentRequiresCredit   = errors.New("card payment requires credit card")
	ErrInvalidCardPaymentAmount    = errors.New("invalid card payment amount")
)

type Card struct {
	ID           int64   `json:"id"`
	Nickname     string  `json:"nickname"`
	Description  string  `json:"description,omitempty"`
	Last4        string  `json:"last4"`
	Brand        string  `json:"brand"`
	CardType     string  `json:"card_type"`
	DueDay       *int    `json:"due_day,omitempty"`
	CreatedAtUTC string  `json:"created_at_utc"`
	UpdatedAtUTC string  `json:"updated_at_utc"`
	DeletedAtUTC *string `json:"deleted_at_utc,omitempty"`
}

type CardDeleteResult struct {
	CardID       int64  `json:"card_id"`
	DeletedAtUTC string `json:"deleted_at_utc"`
}

type CardAddInput struct {
	Nickname    string
	Description string
	Last4       string
	Brand       string
	CardType    string
	DueDay      *int
}

type CardListFilter struct {
	Lookup         string
	CardType       string
	IncludeDeleted bool
}

type CardSelector struct {
	ID       *int64
	Nickname string
	Lookup   string
}

type CardUpdateChanges struct {
	SetNickname    bool
	Nickname       string
	SetDescription bool
	Description    *string
	SetLast4       bool
	Last4          string
	SetBrand       bool
	Brand          string
	SetCardType    bool
	CardType       string
	SetDueDay      bool
	DueDay         *int
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
	DueDay         *int
}

type CardDueInfo struct {
	CardID         int64  `json:"card_id"`
	Nickname       string `json:"nickname"`
	DueDay         int    `json:"due_day"`
	Timezone       string `json:"timezone"`
	ReferenceUTC   string `json:"reference_utc"`
	NextDueDateUTC string `json:"next_due_date_utc"`
}

type CardDebtBalance struct {
	CurrencyCode       string `json:"currency_code"`
	BalanceMinorSigned int64  `json:"balance_minor_signed"`
	State              string `json:"state"`
}

type CardLiabilityEvent struct {
	ID                     int64  `json:"id"`
	CardID                 int64  `json:"card_id"`
	CurrencyCode           string `json:"currency_code"`
	EventType              string `json:"event_type"`
	AmountMinorSigned      int64  `json:"amount_minor_signed"`
	ReferenceTransactionID *int64 `json:"reference_transaction_id,omitempty"`
	Note                   string `json:"note,omitempty"`
	CreatedAtUTC           string `json:"created_at_utc"`
}

type CardPaymentAddInput struct {
	CardID            int64
	CurrencyCode      string
	AmountMinorSigned int64
	Note              string
}

func ValidateCardID(id int64) error {
	if id <= 0 {
		return ErrInvalidCardID
	}
	return nil
}

func NormalizeCardNickname(nickname string) (string, error) {
	normalized := strings.TrimSpace(nickname)
	if normalized == "" {
		return "", ErrCardNicknameRequired
	}
	return normalized, nil
}

func NormalizeCardLast4(last4 string) (string, error) {
	normalized := strings.TrimSpace(last4)
	if len(normalized) != 4 {
		return "", ErrCardLast4Invalid
	}

	for i := 0; i < len(normalized); i++ {
		if normalized[i] < '0' || normalized[i] > '9' {
			return "", ErrCardLast4Invalid
		}
	}
	return normalized, nil
}

func NormalizeCardBrand(brand string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(brand))
	if normalized == "" {
		return "", ErrCardBrandRequired
	}

	replacer := strings.NewReplacer("-", " ", "_", " ", ".", " ")
	collapsed := strings.Join(strings.Fields(replacer.Replace(normalized)), " ")

	switch collapsed {
	case CardBrandVisa:
		return CardBrandVisa, nil
	case "MASTER CARD", CardBrandMastercard:
		return CardBrandMastercard, nil
	case "DINERS CLUB", CardBrandDiners:
		return CardBrandDiners, nil
	case "AMERICAN EXPRESS", CardBrandAmex:
		return CardBrandAmex, nil
	case CardBrandElo:
		return CardBrandElo, nil
	case CardBrandDiscover:
		return CardBrandDiscover, nil
	case CardBrandOther:
		return CardBrandOther, nil
	default:
		return "", ErrInvalidCardBrand
	}
}

func NormalizeCardType(cardType string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(cardType))
	switch normalized {
	case CardTypeCredit, CardTypeDebit:
		return normalized, nil
	default:
		return "", ErrInvalidCardType
	}
}

func ValidateCardDueDay(dueDay int) error {
	if dueDay < 1 || dueDay > 28 {
		return ErrInvalidCardDueDay
	}
	return nil
}

func NormalizeCardAddInput(input CardAddInput) (CardAddInput, error) {
	nickname, err := NormalizeCardNickname(input.Nickname)
	if err != nil {
		return CardAddInput{}, err
	}

	last4, err := NormalizeCardLast4(input.Last4)
	if err != nil {
		return CardAddInput{}, err
	}

	brand, err := NormalizeCardBrand(input.Brand)
	if err != nil {
		return CardAddInput{}, err
	}

	cardType, err := NormalizeCardType(input.CardType)
	if err != nil {
		return CardAddInput{}, err
	}

	description := strings.TrimSpace(input.Description)

	var dueDay *int
	if input.DueDay != nil {
		value := *input.DueDay
		if err := ValidateCardDueDay(value); err != nil {
			return CardAddInput{}, err
		}
		dueDay = &value
	}

	if cardType == CardTypeCredit && dueDay == nil {
		return CardAddInput{}, ErrCardDueDayRequiredForCredit
	}
	if cardType == CardTypeDebit && dueDay != nil {
		return CardAddInput{}, ErrCardDueDayOnlyForCredit
	}

	return CardAddInput{
		Nickname:    nickname,
		Description: description,
		Last4:       last4,
		Brand:       brand,
		CardType:    cardType,
		DueDay:      dueDay,
	}, nil
}

func HasCardUpdateChanges(changes CardUpdateChanges) bool {
	return changes.SetNickname ||
		changes.SetDescription ||
		changes.SetLast4 ||
		changes.SetBrand ||
		changes.SetCardType ||
		changes.SetDueDay
}

func NormalizeCardSelector(selector CardSelector) (CardSelector, error) {
	present := 0

	var normalizedID *int64
	if selector.ID != nil {
		if err := ValidateCardID(*selector.ID); err != nil {
			return CardSelector{}, err
		}
		value := *selector.ID
		normalizedID = &value
		present++
	}

	normalizedNickname := strings.TrimSpace(selector.Nickname)
	if normalizedNickname != "" {
		nickname, err := NormalizeCardNickname(normalizedNickname)
		if err != nil {
			return CardSelector{}, err
		}
		normalizedNickname = nickname
		present++
	}

	normalizedLookup := strings.TrimSpace(selector.Lookup)
	if normalizedLookup != "" {
		present++
	}

	if present == 0 {
		return CardSelector{}, ErrCardLookupRequired
	}
	if present > 1 {
		return CardSelector{}, ErrCardLookupSelectorConflict
	}

	if normalizedLookup != "" {
		lookup, err := NormalizeCardLookupText(normalizedLookup)
		if err != nil {
			return CardSelector{}, err
		}
		normalizedLookup = lookup
	}

	return CardSelector{
		ID:       normalizedID,
		Nickname: normalizedNickname,
		Lookup:   normalizedLookup,
	}, nil
}

func NormalizeCardLookupText(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", ErrInvalidCardLookupText
	}
	return normalized, nil
}

func NextCardDueDateUTC(dueDay int, now time.Time, location *time.Location) (string, error) {
	if err := ValidateCardDueDay(dueDay); err != nil {
		return "", err
	}
	if location == nil {
		location = time.UTC
	}

	localNow := now.In(location)
	year, month, currentDay := localNow.Date()
	nextDue := time.Date(year, month, dueDay, 0, 0, 0, 0, location)
	if currentDay > dueDay {
		nextDue = nextDue.AddDate(0, 1, 0)
	}

	return nextDue.UTC().Format(time.RFC3339Nano), nil
}

func CardDebtState(balanceMinorSigned int64) string {
	switch {
	case balanceMinorSigned > 0:
		return CardDebtStateOwes
	case balanceMinorSigned < 0:
		return CardDebtStateInFavor
	default:
		return CardDebtStateSettled
	}
}

func SortCardsByID(cards []Card) {
	sort.Slice(cards, func(i, j int) bool {
		return cards[i].ID < cards[j].ID
	})
}

func ParseCardDueDay(raw string) (*int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrInvalidCardDueDay, raw)
	}
	if err := ValidateCardDueDay(value); err != nil {
		return nil, err
	}
	return &value, nil
}
