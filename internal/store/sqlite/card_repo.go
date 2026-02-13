package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"boring-budget/internal/ports"
	queries "boring-budget/internal/store/sqlite/sqlc"
)

type CardRepo struct {
	db      *sql.DB
	queries *queries.Queries
	tx      *sql.Tx
}

var _ ports.CardRepository = (*CardRepo)(nil)
var _ ports.CardRepositoryTxBinder = (*CardRepo)(nil)

func NewCardRepo(db *sql.DB) *CardRepo {
	return &CardRepo{
		db:      db,
		queries: queries.New(db),
	}
}

func (r *CardRepo) BindTx(tx *sql.Tx) ports.CardRepository {
	if tx == nil {
		return r
	}

	return &CardRepo{
		db:      r.db,
		queries: r.queries.WithTx(tx),
		tx:      tx,
	}
}

func (r *CardRepo) AddCard(ctx context.Context, input ports.CardCreateInput) (ports.Card, error) {
	if err := validateCardCreateInput(input); err != nil {
		return ports.Card{}, err
	}

	result, err := r.queries.CreateCard(ctx, queries.CreateCardParams{
		Nickname:     strings.TrimSpace(input.Nickname),
		Description:  nullableStringPtr(input.Description),
		Last4:        strings.TrimSpace(input.Last4),
		Brand:        strings.TrimSpace(input.Brand),
		CardType:     strings.TrimSpace(input.CardType),
		DueDay:       nullableInt64Ptr(input.DueDay),
		UpdatedAtUtc: nowRFC3339Nano(),
	})
	if err != nil {
		if isUniqueConstraintErr(err) {
			return ports.Card{}, ports.ErrCardNicknameConflict
		}
		return ports.Card{}, fmt.Errorf("add card: %w", err)
	}

	cardID, err := result.LastInsertId()
	if err != nil {
		return ports.Card{}, fmt.Errorf("add card read id: %w", err)
	}

	return r.GetCardByID(ctx, cardID, false)
}

func (r *CardRepo) GetCardByID(ctx context.Context, id int64, includeDeleted bool) (ports.Card, error) {
	if id <= 0 {
		return ports.Card{}, ports.ErrCardInvalidID
	}

	var row queries.Card
	var err error
	if includeDeleted {
		row, err = r.queries.GetCardByID(ctx, id)
	} else {
		row, err = r.queries.GetActiveCardByID(ctx, id)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ports.Card{}, ports.ErrCardNotFound
		}
		return ports.Card{}, fmt.Errorf("get card by id: %w", err)
	}

	return mapSQLCCard(row), nil
}

func (r *CardRepo) ListCards(ctx context.Context, filter ports.CardListFilter) ([]ports.Card, error) {
	params := queries.ListCardsParams{
		IncludeDeleted: boolAsInt64(filter.IncludeDeleted),
		CardType:       nullableString(filter.CardType),
	}
	rows, err := r.queries.ListCards(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list cards: %w", err)
	}

	out := make([]ports.Card, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapSQLCCard(row))
	}
	return out, nil
}

func (r *CardRepo) SearchCards(ctx context.Context, lookup string, limit int32) ([]ports.Card, error) {
	trimmed := strings.TrimSpace(lookup)
	if trimmed == "" {
		return nil, ports.ErrCardLookupTextRequired
	}
	if limit <= 0 {
		limit = 25
	}

	rows, err := r.queries.SearchActiveCardsByLookup(ctx, queries.SearchActiveCardsByLookupParams{
		LookupText: trimmed,
		LimitRows:  int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("search cards: %w", err)
	}

	out := make([]ports.Card, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapSQLCCard(row))
	}
	return out, nil
}

func (r *CardRepo) UpdateCard(ctx context.Context, input ports.CardUpdateInput) (ports.Card, error) {
	if input.ID <= 0 {
		return ports.Card{}, ports.ErrCardInvalidID
	}
	if err := validateCardTypeDueDay(input.CardType, input.SetDueDay, input.DueDay); err != nil {
		return ports.Card{}, err
	}

	result, err := r.queries.UpdateCardByID(ctx, queries.UpdateCardByIDParams{
		SetNickname:      boolAsInt64(input.Nickname != nil),
		Nickname:         derefString(input.Nickname),
		ClearDescription: boolAsInt64(input.SetDescription && input.Description == nil),
		SetDescription:   boolAsInt64(input.SetDescription && input.Description != nil),
		Description:      nullableStringPtr(input.Description),
		SetLast4:         boolAsInt64(input.Last4 != nil),
		Last4:            derefString(input.Last4),
		SetBrand:         boolAsInt64(input.Brand != nil),
		Brand:            derefString(input.Brand),
		SetCardType:      boolAsInt64(input.CardType != nil),
		CardType:         derefString(input.CardType),
		ClearDueDay:      boolAsInt64(input.SetDueDay && input.DueDay == nil),
		SetDueDay:        boolAsInt64(input.SetDueDay && input.DueDay != nil),
		DueDay:           nullableInt64Ptr(input.DueDay),
		UpdatedAtUtc:     nowRFC3339Nano(),
		ID:               input.ID,
	})
	if err != nil {
		if isUniqueConstraintErr(err) {
			return ports.Card{}, ports.ErrCardNicknameConflict
		}
		return ports.Card{}, fmt.Errorf("update card: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return ports.Card{}, fmt.Errorf("update card rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ports.Card{}, ports.ErrCardNotFound
	}

	return r.GetCardByID(ctx, input.ID, false)
}

func (r *CardRepo) DeleteCard(ctx context.Context, id int64) (ports.CardDeleteResult, error) {
	if id <= 0 {
		return ports.CardDeleteResult{}, ports.ErrCardInvalidID
	}

	deletedAtUTC := nowRFC3339Nano()
	result, err := r.queries.SoftDeleteCard(ctx, queries.SoftDeleteCardParams{
		DeletedAtUtc: sql.NullString{String: deletedAtUTC, Valid: true},
		UpdatedAtUtc: deletedAtUTC,
		ID:           id,
	})
	if err != nil {
		return ports.CardDeleteResult{}, fmt.Errorf("delete card: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return ports.CardDeleteResult{}, fmt.Errorf("delete card rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ports.CardDeleteResult{}, ports.ErrCardNotFound
	}

	return ports.CardDeleteResult{
		CardID:       id,
		DeletedAtUTC: deletedAtUTC,
	}, nil
}

func (r *CardRepo) GetCardDue(ctx context.Context, cardID int64, asOfDate string) (ports.CardDue, error) {
	if cardID <= 0 {
		return ports.CardDue{}, ports.ErrCardInvalidID
	}

	asOf, err := parseAsOfDate(asOfDate)
	if err != nil {
		return ports.CardDue{}, err
	}

	row, err := r.queries.GetActiveCardDueByID(ctx, cardID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ports.CardDue{}, ports.ErrCardNotFound
		}
		return ports.CardDue{}, fmt.Errorf("get card due: %w", err)
	}
	if !row.DueDay.Valid {
		return ports.CardDue{}, ports.ErrCardDueDayRequired
	}

	nextDue := computeNextDueDate(asOf, int(row.DueDay.Int64))
	return ports.CardDue{
		CardID:      row.ID,
		Nickname:    row.Nickname,
		DueDay:      row.DueDay.Int64,
		AsOfDate:    asOf.Format("2006-01-02"),
		NextDueDate: nextDue.Format(time.RFC3339Nano),
	}, nil
}

func (r *CardRepo) ListCardDues(ctx context.Context, asOfDate string) ([]ports.CardDue, error) {
	asOf, err := parseAsOfDate(asOfDate)
	if err != nil {
		return nil, err
	}

	rows, err := r.queries.ListActiveCreditCardDues(ctx)
	if err != nil {
		return nil, fmt.Errorf("list card dues: %w", err)
	}

	out := make([]ports.CardDue, 0, len(rows))
	for _, row := range rows {
		if !row.DueDay.Valid {
			continue
		}
		nextDue := computeNextDueDate(asOf, int(row.DueDay.Int64))
		out = append(out, ports.CardDue{
			CardID:      row.ID,
			Nickname:    row.Nickname,
			DueDay:      row.DueDay.Int64,
			AsOfDate:    asOf.Format("2006-01-02"),
			NextDueDate: nextDue.Format(time.RFC3339Nano),
		})
	}

	return out, nil
}

func (r *CardRepo) UpsertTransactionPaymentMethod(ctx context.Context, input ports.TransactionPaymentMethodUpsertInput) (ports.TransactionPaymentMethod, error) {
	if input.TransactionID <= 0 {
		return ports.TransactionPaymentMethod{}, ports.ErrTransactionInvalidID
	}
	if strings.TrimSpace(input.MethodType) == "" {
		return ports.TransactionPaymentMethod{}, ports.ErrPaymentMethodInvalid
	}
	if err := validatePaymentMethod(input.MethodType, input.CardID); err != nil {
		return ports.TransactionPaymentMethod{}, err
	}

	exists, err := r.queries.ExistsTransactionByID(ctx, input.TransactionID)
	if err != nil {
		return ports.TransactionPaymentMethod{}, fmt.Errorf("upsert payment method check transaction: %w", err)
	}
	if !isTruthy(exists) {
		return ports.TransactionPaymentMethod{}, ports.ErrTransactionNotFound
	}

	if input.CardID != nil {
		existsCard, err := r.queries.ExistsActiveCardByID(ctx, *input.CardID)
		if err != nil {
			return ports.TransactionPaymentMethod{}, fmt.Errorf("upsert payment method check card: %w", err)
		}
		if !isTruthy(existsCard) {
			return ports.TransactionPaymentMethod{}, ports.ErrCardNotFound
		}
	}

	_, err = r.queries.UpsertTransactionPaymentMethod(ctx, queries.UpsertTransactionPaymentMethodParams{
		TransactionID: input.TransactionID,
		MethodType:    strings.ToLower(strings.TrimSpace(input.MethodType)),
		CardID:        nullableInt64(input.CardID),
		UpdatedAtUtc:  nowRFC3339Nano(),
	})
	if err != nil {
		return ports.TransactionPaymentMethod{}, fmt.Errorf("upsert payment method: %w", err)
	}

	return r.GetTransactionPaymentMethod(ctx, input.TransactionID)
}

func (r *CardRepo) GetTransactionPaymentMethod(ctx context.Context, transactionID int64) (ports.TransactionPaymentMethod, error) {
	if transactionID <= 0 {
		return ports.TransactionPaymentMethod{}, ports.ErrTransactionInvalidID
	}

	row, err := r.queries.GetTransactionPaymentMethodByTransactionID(ctx, transactionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ports.TransactionPaymentMethod{}, ports.ErrTransactionPaymentMethodNotFound
		}
		return ports.TransactionPaymentMethod{}, fmt.Errorf("get transaction payment method: %w", err)
	}

	return ports.TransactionPaymentMethod{
		TransactionID: row.TransactionID,
		MethodType:    row.MethodType,
		CardID:        ptrInt64FromNull(row.CardID),
		CreatedAtUTC:  row.CreatedAtUtc,
		UpdatedAtUTC:  row.UpdatedAtUtc,
	}, nil
}

func (r *CardRepo) AddLiabilityEvent(ctx context.Context, input ports.CreditLiabilityEventInput) (ports.CreditLiabilityEvent, error) {
	if err := validateLiabilityEventInput(input); err != nil {
		return ports.CreditLiabilityEvent{}, err
	}

	result, err := r.queries.CreateCreditLiabilityEvent(ctx, queries.CreateCreditLiabilityEventParams{
		CardID:                 input.CardID,
		CurrencyCode:           strings.ToUpper(strings.TrimSpace(input.CurrencyCode)),
		EventType:              strings.ToLower(strings.TrimSpace(input.EventType)),
		AmountMinorSigned:      input.AmountMinorSigned,
		ReferenceTransactionID: nullableInt64(input.ReferenceTransactionID),
		Note:                   nullableStringPtr(input.Note),
		CreatedAtUtc:           nowRFC3339Nano(),
	})
	if err != nil {
		return ports.CreditLiabilityEvent{}, fmt.Errorf("add liability event: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return ports.CreditLiabilityEvent{}, fmt.Errorf("add liability event read id: %w", err)
	}

	row, err := r.queries.GetCreditLiabilityEventByID(ctx, id)
	if err != nil {
		return ports.CreditLiabilityEvent{}, fmt.Errorf("load liability event: %w", err)
	}
	return mapSQLCCreditLiabilityEvent(row), nil
}

func (r *CardRepo) AddPaymentEvent(ctx context.Context, input ports.CardPaymentEventInput) (ports.CreditLiabilityEvent, error) {
	if input.CardID <= 0 {
		return ports.CreditLiabilityEvent{}, ports.ErrCardInvalidID
	}
	if err := validateCurrencyCode(input.CurrencyCode); err != nil {
		return ports.CreditLiabilityEvent{}, err
	}
	if input.AmountMinor <= 0 {
		return ports.CreditLiabilityEvent{}, ports.ErrLiabilityAmountInvalid
	}

	card, err := r.GetCardByID(ctx, input.CardID, false)
	if err != nil {
		return ports.CreditLiabilityEvent{}, err
	}
	if card.CardType != ports.CardTypeCredit {
		return ports.CreditLiabilityEvent{}, ports.ErrCardInvalidType
	}

	return r.AddLiabilityEvent(ctx, ports.CreditLiabilityEventInput{
		CardID:                 input.CardID,
		CurrencyCode:           strings.ToUpper(strings.TrimSpace(input.CurrencyCode)),
		EventType:              ports.LiabilityEventPayment,
		AmountMinorSigned:      -input.AmountMinor,
		ReferenceTransactionID: input.ReferenceTransactionID,
		Note:                   input.Note,
	})
}

func (r *CardRepo) ListLiabilityEvents(ctx context.Context, cardID int64, currencyCode string) ([]ports.CreditLiabilityEvent, error) {
	if cardID <= 0 {
		return nil, ports.ErrCardInvalidID
	}

	var rows []queries.CreditLiabilityEvent
	var err error
	trimmedCurrency := strings.TrimSpace(currencyCode)
	if trimmedCurrency == "" {
		rows, err = r.queries.ListCreditLiabilityEventsByCard(ctx, cardID)
	} else {
		if err := validateCurrencyCode(trimmedCurrency); err != nil {
			return nil, err
		}
		rows, err = r.queries.ListCreditLiabilityEventsByCardAndCurrency(ctx, queries.ListCreditLiabilityEventsByCardAndCurrencyParams{
			CardID:       cardID,
			CurrencyCode: strings.ToUpper(trimmedCurrency),
		})
	}
	if err != nil {
		return nil, fmt.Errorf("list liability events: %w", err)
	}

	out := make([]ports.CreditLiabilityEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapSQLCCreditLiabilityEvent(row))
	}
	return out, nil
}

func (r *CardRepo) GetDebtSummaryByCard(ctx context.Context, cardID int64) ([]ports.CardDebtBucket, error) {
	if cardID <= 0 {
		return nil, ports.ErrCardInvalidID
	}

	rows, err := r.queries.ListCreditLiabilitySummaryByCard(ctx, cardID)
	if err != nil {
		return nil, fmt.Errorf("get debt summary by card: %w", err)
	}

	out := make([]ports.CardDebtBucket, 0, len(rows))
	for _, row := range rows {
		out = append(out, ports.CardDebtBucket{
			CardID:         row.CardID,
			CurrencyCode:   row.CurrencyCode,
			BalanceMinor:   row.BalanceMinor,
			State:          debtState(row.BalanceMinor),
			LastEventAtUTC: stringFromInterface(row.LastEventAtUtc),
		})
	}
	return out, nil
}

func (r *CardRepo) GetDebtSummary(ctx context.Context) ([]ports.CardDebtBucket, error) {
	rows, err := r.queries.ListCreditLiabilitySummaryAllCards(ctx)
	if err != nil {
		return nil, fmt.Errorf("get debt summary: %w", err)
	}

	out := make([]ports.CardDebtBucket, 0, len(rows))
	for _, row := range rows {
		out = append(out, ports.CardDebtBucket{
			CardID:         row.CardID,
			CurrencyCode:   row.CurrencyCode,
			BalanceMinor:   row.BalanceMinor,
			State:          debtState(row.BalanceMinor),
			LastEventAtUTC: stringFromInterface(row.LastEventAtUtc),
		})
	}
	return out, nil
}

func (r *CardRepo) GetDebtBalance(ctx context.Context, cardID int64, currencyCode string) (int64, error) {
	if cardID <= 0 {
		return 0, ports.ErrCardInvalidID
	}
	if err := validateCurrencyCode(currencyCode); err != nil {
		return 0, err
	}

	return r.queries.GetCreditLiabilityBalanceByCardAndCurrency(ctx, queries.GetCreditLiabilityBalanceByCardAndCurrencyParams{
		CardID:       cardID,
		CurrencyCode: strings.ToUpper(strings.TrimSpace(currencyCode)),
	})
}

func validateCardCreateInput(input ports.CardCreateInput) error {
	if strings.TrimSpace(input.Nickname) == "" {
		return ports.ErrCardNicknameRequired
	}
	if strings.TrimSpace(input.Last4) == "" || len(strings.TrimSpace(input.Last4)) != 4 {
		return ports.ErrCardInvalidLast4
	}
	for _, ch := range strings.TrimSpace(input.Last4) {
		if ch < '0' || ch > '9' {
			return ports.ErrCardInvalidLast4
		}
	}
	if strings.TrimSpace(input.Brand) == "" {
		return ports.ErrCardBrandRequired
	}

	cardType := strings.ToLower(strings.TrimSpace(input.CardType))
	if cardType != ports.CardTypeCredit && cardType != ports.CardTypeDebit {
		return ports.ErrCardInvalidType
	}

	if input.DueDay != nil {
		if *input.DueDay < 1 || *input.DueDay > 28 {
			return ports.ErrCardInvalidDueDay
		}
	}
	if cardType == ports.CardTypeCredit && input.DueDay == nil {
		return ports.ErrCardDueDayRequired
	}
	if cardType == ports.CardTypeDebit && input.DueDay != nil {
		return ports.ErrCardDueDayNotAllowed
	}

	return nil
}

func validateCardTypeDueDay(cardType *string, setDueDay bool, dueDay *int64) error {
	if cardType == nil {
		return nil
	}
	normalizedType := strings.ToLower(strings.TrimSpace(*cardType))
	if normalizedType != ports.CardTypeCredit && normalizedType != ports.CardTypeDebit {
		return ports.ErrCardInvalidType
	}
	if !setDueDay {
		return nil
	}
	if dueDay != nil && (*dueDay < 1 || *dueDay > 28) {
		return ports.ErrCardInvalidDueDay
	}
	if normalizedType == ports.CardTypeCredit && dueDay == nil {
		return ports.ErrCardDueDayRequired
	}
	if normalizedType == ports.CardTypeDebit && dueDay != nil {
		return ports.ErrCardDueDayNotAllowed
	}
	return nil
}

func validatePaymentMethod(method string, cardID *int64) error {
	normalized := strings.ToLower(strings.TrimSpace(method))
	switch normalized {
	case ports.PaymentMethodCash:
		if cardID != nil {
			return ports.ErrPaymentMethodCardNotAllowed
		}
	case ports.PaymentMethodCard:
		if cardID == nil {
			return ports.ErrPaymentMethodCardRequired
		}
		if *cardID <= 0 {
			return ports.ErrCardInvalidID
		}
	default:
		return ports.ErrPaymentMethodInvalid
	}
	return nil
}

func validateLiabilityEventInput(input ports.CreditLiabilityEventInput) error {
	if input.CardID <= 0 {
		return ports.ErrCardInvalidID
	}
	if err := validateCurrencyCode(input.CurrencyCode); err != nil {
		return err
	}
	eventType := strings.ToLower(strings.TrimSpace(input.EventType))
	if eventType != ports.LiabilityEventCharge && eventType != ports.LiabilityEventPayment && eventType != ports.LiabilityEventAdjustment {
		return ports.ErrLiabilityEventTypeInvalid
	}
	if input.AmountMinorSigned == 0 {
		return ports.ErrLiabilityAmountInvalid
	}
	if eventType == ports.LiabilityEventCharge && input.AmountMinorSigned <= 0 {
		return ports.ErrLiabilityAmountInvalid
	}
	if eventType == ports.LiabilityEventPayment && input.AmountMinorSigned >= 0 {
		return ports.ErrLiabilityAmountInvalid
	}
	if input.ReferenceTransactionID != nil && *input.ReferenceTransactionID <= 0 {
		return ports.ErrTransactionInvalidID
	}
	return nil
}

func validateCurrencyCode(code string) error {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	if len(normalized) != 3 {
		return ports.ErrCurrencyCodeInvalid
	}
	for _, ch := range normalized {
		if ch < 'A' || ch > 'Z' {
			return ports.ErrCurrencyCodeInvalid
		}
	}
	return nil
}

func parseAsOfDate(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Now().UTC(), nil
	}

	if parsed, err := time.Parse("2006-01-02", trimmed); err == nil {
		return parsed.UTC(), nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return parsed.UTC(), nil
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed.UTC(), nil
	}

	return time.Time{}, ports.ErrCardInvalidAsOfDate
}

func computeNextDueDate(asOf time.Time, dueDay int) time.Time {
	year, month, day := asOf.UTC().Date()
	next := time.Date(year, month, dueDay, 0, 0, 0, 0, time.UTC)
	if day > dueDay {
		next = next.AddDate(0, 1, 0)
	}
	return next
}

func mapSQLCCard(row queries.Card) ports.Card {
	return ports.Card{
		ID:           row.ID,
		Nickname:     row.Nickname,
		Description:  ptrStringFromNull(row.Description),
		Last4:        row.Last4,
		Brand:        row.Brand,
		CardType:     row.CardType,
		DueDay:       ptrInt64FromNull(row.DueDay),
		CreatedAtUTC: row.CreatedAtUtc,
		UpdatedAtUTC: row.UpdatedAtUtc,
		DeletedAtUTC: ptrStringFromNull(row.DeletedAtUtc),
	}
}

func mapSQLCCreditLiabilityEvent(row queries.CreditLiabilityEvent) ports.CreditLiabilityEvent {
	return ports.CreditLiabilityEvent{
		ID:                     row.ID,
		CardID:                 row.CardID,
		CurrencyCode:           row.CurrencyCode,
		EventType:              row.EventType,
		AmountMinorSigned:      row.AmountMinorSigned,
		ReferenceTransactionID: ptrInt64FromNull(row.ReferenceTransactionID),
		Note:                   ptrStringFromNull(row.Note),
		CreatedAtUTC:           row.CreatedAtUtc,
	}
}

func ptrInt64FromNull(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	out := value.Int64
	return &out
}

func ptrStringFromNull(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	out := value.String
	return &out
}

func nullableStringPtr(value *string) sql.NullString {
	if value == nil || strings.TrimSpace(*value) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: strings.TrimSpace(*value), Valid: true}
}

func nullableInt64Ptr(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func boolAsInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func nowRFC3339Nano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func stringFromInterface(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func debtState(balance int64) string {
	switch {
	case balance > 0:
		return ports.LiabilityStateOwes
	case balance < 0:
		return ports.LiabilityStateInFavor
	default:
		return ports.LiabilityStateSettled
	}
}
