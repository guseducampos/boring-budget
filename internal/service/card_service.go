package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"boring-budget/internal/domain"
	"boring-budget/internal/ports"
)

const (
	cardLookupSearchLimit = int32(25)
)

type CardRepository = ports.CardRepository

type CardService struct {
	repo CardRepository
}

type CardLookupConflictError struct {
	Lookup     string
	Candidates []domain.Card
}

func (e *CardLookupConflictError) Error() string {
	if e == nil {
		return domain.ErrCardLookupAmbiguous.Error()
	}
	return fmt.Sprintf("%s: %q", domain.ErrCardLookupAmbiguous.Error(), e.Lookup)
}

func (e *CardLookupConflictError) Unwrap() error {
	return domain.ErrCardLookupAmbiguous
}

type CardDebtCardSummary struct {
	Card    domain.Card              `json:"card"`
	Buckets []domain.CardDebtBalance `json:"buckets"`
}

type CardPaymentResult struct {
	Card    domain.Card               `json:"card"`
	Event   domain.CardLiabilityEvent `json:"event"`
	Balance domain.CardDebtBalance    `json:"balance"`
}

func NewCardService(repo CardRepository) (*CardService, error) {
	if repo == nil {
		return nil, fmt.Errorf("card service: repo is required")
	}
	return &CardService{repo: repo}, nil
}

func (s *CardService) Add(ctx context.Context, input domain.CardAddInput) (domain.Card, error) {
	normalized, err := domain.NormalizeCardAddInput(input)
	if err != nil {
		return domain.Card{}, err
	}

	var description *string
	if normalized.Description != "" {
		value := normalized.Description
		description = &value
	}

	var dueDay *int64
	if normalized.DueDay != nil {
		value := int64(*normalized.DueDay)
		dueDay = &value
	}

	card, err := s.repo.AddCard(ctx, ports.CardCreateInput{
		Nickname:    normalized.Nickname,
		Description: description,
		Last4:       normalized.Last4,
		Brand:       normalized.Brand,
		CardType:    normalized.CardType,
		DueDay:      dueDay,
	})
	if err != nil {
		return domain.Card{}, mapCardRepoError(err)
	}

	return fromPortsCard(card), nil
}

func (s *CardService) List(ctx context.Context, filter domain.CardListFilter) ([]domain.Card, error) {
	lookup := strings.TrimSpace(filter.Lookup)
	if lookup != "" {
		normalizedLookup, err := domain.NormalizeCardLookupText(lookup)
		if err != nil {
			return nil, err
		}

		matches, err := s.repo.SearchCards(ctx, normalizedLookup, cardLookupSearchLimit)
		if err != nil {
			return nil, mapCardRepoError(err)
		}
		cards := fromPortsCards(matches)
		sortCardsDeterministic(cards)
		return cards, nil
	}

	listFilter := ports.CardListFilter{
		IncludeDeleted: filter.IncludeDeleted,
	}
	if strings.TrimSpace(filter.CardType) != "" {
		cardType, err := domain.NormalizeCardType(filter.CardType)
		if err != nil {
			return nil, err
		}
		listFilter.CardType = cardType
	}

	cards, err := s.repo.ListCards(ctx, listFilter)
	if err != nil {
		return nil, mapCardRepoError(err)
	}

	result := fromPortsCards(cards)
	sortCardsDeterministic(result)
	return result, nil
}

func (s *CardService) Resolve(ctx context.Context, selector domain.CardSelector) (domain.Card, error) {
	normalizedSelector, err := domain.NormalizeCardSelector(selector)
	if err != nil {
		return domain.Card{}, err
	}

	if normalizedSelector.ID != nil {
		card, err := s.repo.GetCardByID(ctx, *normalizedSelector.ID, false)
		if err != nil {
			return domain.Card{}, mapCardRepoError(err)
		}
		return fromPortsCard(card), nil
	}

	if normalizedSelector.Nickname != "" {
		cards, err := s.repo.ListCards(ctx, ports.CardListFilter{IncludeDeleted: false})
		if err != nil {
			return domain.Card{}, mapCardRepoError(err)
		}

		matches := make([]ports.Card, 0, 1)
		for _, card := range cards {
			if strings.EqualFold(card.Nickname, normalizedSelector.Nickname) {
				matches = append(matches, card)
			}
		}
		if len(matches) == 0 {
			return domain.Card{}, domain.ErrCardNotFound
		}
		if len(matches) > 1 {
			candidates := fromPortsCards(matches)
			sortCardsDeterministic(candidates)
			return domain.Card{}, &CardLookupConflictError{
				Lookup:     normalizedSelector.Nickname,
				Candidates: candidates,
			}
		}
		return fromPortsCard(matches[0]), nil
	}

	matches, err := s.repo.SearchCards(ctx, normalizedSelector.Lookup, cardLookupSearchLimit)
	if err != nil {
		return domain.Card{}, mapCardRepoError(err)
	}
	if len(matches) == 0 {
		return domain.Card{}, domain.ErrCardNotFound
	}
	if len(matches) > 1 {
		candidates := fromPortsCards(matches)
		sortCardsDeterministic(candidates)
		return domain.Card{}, &CardLookupConflictError{
			Lookup:     normalizedSelector.Lookup,
			Candidates: candidates,
		}
	}
	return fromPortsCard(matches[0]), nil
}

func (s *CardService) Update(ctx context.Context, input domain.CardUpdateInput) (domain.Card, error) {
	if err := domain.ValidateCardID(input.ID); err != nil {
		return domain.Card{}, err
	}
	if !hasCardUpdateInputChanges(input) {
		return domain.Card{}, domain.ErrNoCardUpdateFields
	}

	currentRaw, err := s.repo.GetCardByID(ctx, input.ID, false)
	if err != nil {
		return domain.Card{}, mapCardRepoError(err)
	}
	current := fromPortsCard(currentRaw)

	normalized := ports.CardUpdateInput{
		ID: input.ID,
	}

	finalType := current.CardType
	finalDueDay := current.DueDay

	if input.Nickname != nil {
		value, err := domain.NormalizeCardNickname(*input.Nickname)
		if err != nil {
			return domain.Card{}, err
		}
		normalized.Nickname = &value
	}

	if input.SetDescription {
		normalized.SetDescription = true
		if input.Description != nil {
			value := strings.TrimSpace(*input.Description)
			if value != "" {
				normalized.Description = &value
			}
		}
	}

	if input.Last4 != nil {
		value, err := domain.NormalizeCardLast4(*input.Last4)
		if err != nil {
			return domain.Card{}, err
		}
		normalized.Last4 = &value
	}

	if input.Brand != nil {
		value, err := domain.NormalizeCardBrand(*input.Brand)
		if err != nil {
			return domain.Card{}, err
		}
		normalized.Brand = &value
	}

	if input.CardType != nil {
		value, err := domain.NormalizeCardType(*input.CardType)
		if err != nil {
			return domain.Card{}, err
		}
		normalized.CardType = &value
		finalType = value
	}

	if input.SetDueDay {
		normalized.SetDueDay = true
		finalDueDay = nil
		if input.DueDay != nil {
			if err := domain.ValidateCardDueDay(*input.DueDay); err != nil {
				return domain.Card{}, err
			}
			value := int64(*input.DueDay)
			normalized.DueDay = &value
			intValue := int(value)
			finalDueDay = &intValue
		}
	}

	if finalType == domain.CardTypeCredit && finalDueDay == nil {
		return domain.Card{}, domain.ErrCardDueDayRequiredForCredit
	}
	if finalType == domain.CardTypeDebit && finalDueDay != nil {
		return domain.Card{}, domain.ErrCardDueDayOnlyForCredit
	}

	card, err := s.repo.UpdateCard(ctx, normalized)
	if err != nil {
		return domain.Card{}, mapCardRepoError(err)
	}
	return fromPortsCard(card), nil
}

func (s *CardService) Delete(ctx context.Context, id int64) (domain.CardDeleteResult, error) {
	if err := domain.ValidateCardID(id); err != nil {
		return domain.CardDeleteResult{}, err
	}

	result, err := s.repo.DeleteCard(ctx, id)
	if err != nil {
		return domain.CardDeleteResult{}, mapCardRepoError(err)
	}
	return domain.CardDeleteResult{
		CardID:       result.CardID,
		DeletedAtUTC: result.DeletedAtUTC,
	}, nil
}

func (s *CardService) ShowDue(ctx context.Context, id int64, asOfDate string, timezone string) (domain.CardDueInfo, error) {
	if err := domain.ValidateCardID(id); err != nil {
		return domain.CardDueInfo{}, err
	}
	normalizedAsOf, err := normalizeAsOfDate(asOfDate)
	if err != nil {
		return domain.CardDueInfo{}, err
	}

	due, err := s.repo.GetCardDue(ctx, id, normalizedAsOf)
	if err != nil {
		return domain.CardDueInfo{}, mapCardRepoError(err)
	}

	return domain.CardDueInfo{
		CardID:         due.CardID,
		Nickname:       due.Nickname,
		DueDay:         int(due.DueDay),
		Timezone:       strings.TrimSpace(timezone),
		ReferenceUTC:   due.AsOfDate,
		NextDueDateUTC: due.NextDueDate,
	}, nil
}

func (s *CardService) ListDues(ctx context.Context, asOfDate string, timezone string) ([]domain.CardDueInfo, error) {
	normalizedAsOf, err := normalizeAsOfDate(asOfDate)
	if err != nil {
		return nil, err
	}

	rows, err := s.repo.ListCardDues(ctx, normalizedAsOf)
	if err != nil {
		return nil, mapCardRepoError(err)
	}

	dues := make([]domain.CardDueInfo, 0, len(rows))
	for _, row := range rows {
		dues = append(dues, domain.CardDueInfo{
			CardID:         row.CardID,
			Nickname:       row.Nickname,
			DueDay:         int(row.DueDay),
			Timezone:       strings.TrimSpace(timezone),
			ReferenceUTC:   row.AsOfDate,
			NextDueDateUTC: row.NextDueDate,
		})
	}

	sort.Slice(dues, func(i, j int) bool {
		if dues[i].DueDay != dues[j].DueDay {
			return dues[i].DueDay < dues[j].DueDay
		}
		return dues[i].CardID < dues[j].CardID
	})
	return dues, nil
}

func (s *CardService) ShowDebtByCard(ctx context.Context, id int64) (CardDebtCardSummary, error) {
	if err := domain.ValidateCardID(id); err != nil {
		return CardDebtCardSummary{}, err
	}

	cardRaw, err := s.repo.GetCardByID(ctx, id, false)
	if err != nil {
		return CardDebtCardSummary{}, mapCardRepoError(err)
	}

	buckets, err := s.repo.GetDebtSummaryByCard(ctx, id)
	if err != nil {
		return CardDebtCardSummary{}, mapCardRepoError(err)
	}

	return CardDebtCardSummary{
		Card:    fromPortsCard(cardRaw),
		Buckets: fromPortsDebtBuckets(buckets),
	}, nil
}

func (s *CardService) ShowDebtAll(ctx context.Context) ([]CardDebtCardSummary, error) {
	rows, err := s.repo.GetDebtSummary(ctx)
	if err != nil {
		return nil, mapCardRepoError(err)
	}

	if len(rows) == 0 {
		return []CardDebtCardSummary{}, nil
	}

	cardIDs := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		cardIDs[row.CardID] = struct{}{}
	}

	cardByID := make(map[int64]domain.Card, len(cardIDs))
	for cardID := range cardIDs {
		cardRaw, err := s.repo.GetCardByID(ctx, cardID, false)
		if err != nil {
			return nil, mapCardRepoError(err)
		}
		cardByID[cardID] = fromPortsCard(cardRaw)
	}

	summaryByCard := make(map[int64][]domain.CardDebtBalance, len(cardIDs))
	for _, row := range rows {
		summaryByCard[row.CardID] = append(summaryByCard[row.CardID], domain.CardDebtBalance{
			CurrencyCode:       row.CurrencyCode,
			BalanceMinorSigned: row.BalanceMinor,
			State:              domain.CardDebtState(row.BalanceMinor),
		})
	}

	out := make([]CardDebtCardSummary, 0, len(cardIDs))
	for cardID, buckets := range summaryByCard {
		sort.Slice(buckets, func(i, j int) bool {
			return buckets[i].CurrencyCode < buckets[j].CurrencyCode
		})
		out = append(out, CardDebtCardSummary{
			Card:    cardByID[cardID],
			Buckets: buckets,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Card.ID < out[j].Card.ID
	})
	return out, nil
}

func (s *CardService) AddPayment(ctx context.Context, input domain.CardPaymentAddInput) (CardPaymentResult, error) {
	if err := domain.ValidateCardID(input.CardID); err != nil {
		return CardPaymentResult{}, err
	}
	if input.AmountMinorSigned <= 0 {
		return CardPaymentResult{}, domain.ErrInvalidCardPaymentAmount
	}

	normalizedCurrency, err := domain.NormalizeCurrencyCode(input.CurrencyCode)
	if err != nil {
		return CardPaymentResult{}, err
	}

	cardRaw, err := s.repo.GetCardByID(ctx, input.CardID, false)
	if err != nil {
		return CardPaymentResult{}, mapCardRepoError(err)
	}
	card := fromPortsCard(cardRaw)
	if card.CardType != domain.CardTypeCredit {
		return CardPaymentResult{}, domain.ErrCardPaymentRequiresCredit
	}

	note := strings.TrimSpace(input.Note)
	var notePtr *string
	if note != "" {
		notePtr = &note
	}

	eventRaw, err := s.repo.AddPaymentEvent(ctx, ports.CardPaymentEventInput{
		CardID:       input.CardID,
		CurrencyCode: normalizedCurrency,
		AmountMinor:  input.AmountMinorSigned,
		Note:         notePtr,
	})
	if err != nil {
		return CardPaymentResult{}, mapCardRepoError(err)
	}

	balance, err := s.repo.GetDebtBalance(ctx, input.CardID, normalizedCurrency)
	if err != nil {
		return CardPaymentResult{}, mapCardRepoError(err)
	}

	return CardPaymentResult{
		Card:  card,
		Event: fromPortsLiabilityEvent(eventRaw),
		Balance: domain.CardDebtBalance{
			CurrencyCode:       normalizedCurrency,
			BalanceMinorSigned: balance,
			State:              domain.CardDebtState(balance),
		},
	}, nil
}

func hasCardUpdateInputChanges(input domain.CardUpdateInput) bool {
	return input.Nickname != nil ||
		input.SetDescription ||
		input.Last4 != nil ||
		input.Brand != nil ||
		input.CardType != nil ||
		input.SetDueDay
}

func normalizeAsOfDate(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Now().UTC().Format("2006-01-02"), nil
	}

	if parsed, err := time.Parse("2006-01-02", trimmed); err == nil {
		return parsed.UTC().Format("2006-01-02"), nil
	}

	if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return parsed.UTC().Format("2006-01-02"), nil
	}

	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed.UTC().Format("2006-01-02"), nil
	}

	return "", domain.ErrInvalidCardAsOfDate
}

func fromPortsCard(card ports.Card) domain.Card {
	out := domain.Card{
		ID:           card.ID,
		Nickname:     card.Nickname,
		Last4:        card.Last4,
		Brand:        card.Brand,
		CardType:     card.CardType,
		CreatedAtUTC: card.CreatedAtUTC,
		UpdatedAtUTC: card.UpdatedAtUTC,
		DeletedAtUTC: card.DeletedAtUTC,
	}
	if card.Description != nil {
		out.Description = *card.Description
	}
	if card.DueDay != nil {
		value := int(*card.DueDay)
		out.DueDay = &value
	}
	return out
}

func fromPortsCards(cards []ports.Card) []domain.Card {
	if len(cards) == 0 {
		return []domain.Card{}
	}
	out := make([]domain.Card, 0, len(cards))
	for _, card := range cards {
		out = append(out, fromPortsCard(card))
	}
	return out
}

func sortCardsDeterministic(cards []domain.Card) {
	sort.Slice(cards, func(i, j int) bool {
		left := strings.ToLower(cards[i].Nickname)
		right := strings.ToLower(cards[j].Nickname)
		if left != right {
			return left < right
		}
		return cards[i].ID < cards[j].ID
	})
}

func fromPortsLiabilityEvent(event ports.CreditLiabilityEvent) domain.CardLiabilityEvent {
	out := domain.CardLiabilityEvent{
		ID:                     event.ID,
		CardID:                 event.CardID,
		CurrencyCode:           event.CurrencyCode,
		EventType:              event.EventType,
		AmountMinorSigned:      event.AmountMinorSigned,
		CreatedAtUTC:           event.CreatedAtUTC,
		ReferenceTransactionID: event.ReferenceTransactionID,
	}
	if event.Note != nil {
		out.Note = *event.Note
	}
	return out
}

func fromPortsDebtBuckets(rows []ports.CardDebtBucket) []domain.CardDebtBalance {
	if len(rows) == 0 {
		return []domain.CardDebtBalance{}
	}

	out := make([]domain.CardDebtBalance, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.CardDebtBalance{
			CurrencyCode:       row.CurrencyCode,
			BalanceMinorSigned: row.BalanceMinor,
			State:              domain.CardDebtState(row.BalanceMinor),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CurrencyCode < out[j].CurrencyCode
	})
	return out
}

func mapCardRepoError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, ports.ErrCardInvalidID):
		return domain.ErrInvalidCardID
	case errors.Is(err, ports.ErrCardNicknameRequired):
		return domain.ErrCardNicknameRequired
	case errors.Is(err, ports.ErrCardNicknameConflict):
		return domain.ErrCardNicknameConflict
	case errors.Is(err, ports.ErrCardNotFound):
		return domain.ErrCardNotFound
	case errors.Is(err, ports.ErrCardInvalidLast4):
		return domain.ErrCardLast4Invalid
	case errors.Is(err, ports.ErrCardBrandRequired):
		return domain.ErrCardBrandRequired
	case errors.Is(err, ports.ErrCardInvalidType):
		return domain.ErrInvalidCardType
	case errors.Is(err, ports.ErrCardInvalidDueDay):
		return domain.ErrInvalidCardDueDay
	case errors.Is(err, ports.ErrCardDueDayRequired):
		return domain.ErrCardDueDayRequiredForCredit
	case errors.Is(err, ports.ErrCardDueDayNotAllowed):
		return domain.ErrCardDueDayOnlyForCredit
	case errors.Is(err, ports.ErrCardInvalidAsOfDate):
		return domain.ErrInvalidCardAsOfDate
	case errors.Is(err, ports.ErrCardLookupTextRequired):
		return domain.ErrInvalidCardLookupText
	case errors.Is(err, ports.ErrCurrencyCodeInvalid):
		return domain.ErrInvalidCurrencyCode
	case errors.Is(err, ports.ErrLiabilityAmountInvalid):
		return domain.ErrInvalidCardPaymentAmount
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed") {
		return domain.ErrCardNicknameConflict
	}

	return err
}
