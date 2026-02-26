package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"boring-budget/internal/domain"
)

type SavingsEntryReader interface {
	List(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error)
}

type SavingsEventRepository interface {
	AddEvent(ctx context.Context, input domain.SavingsEventAddInput) (domain.SavingsEvent, error)
	ListEvents(ctx context.Context, filter domain.SavingsEventListFilter) ([]domain.SavingsEvent, error)
}

type SavingsBalanceLinkReader interface {
	ListBalanceLinks(ctx context.Context) ([]domain.BalanceAccountLink, error)
}

type SavingsService struct {
	entryReader SavingsEntryReader
	eventRepo   SavingsEventRepository
	linkReader  SavingsBalanceLinkReader
}

type SavingsAddInput struct {
	AmountMinor              int64
	CurrencyCode             string
	EventDateUTC             string
	SourceBankAccountID      *int64
	DestinationBankAccountID *int64
	Note                     string
}

type SavingsShowRequest struct {
	IncludeLifetime bool
	IncludeRange    bool
	RangeFromUTC    string
	RangeToUTC      string
}

type savingsReplayKind int

const (
	savingsReplayKindEntry savingsReplayKind = iota
	savingsReplayKindEvent
)

type savingsReplayItem struct {
	kind        savingsReplayKind
	id          int64
	date        time.Time
	currency    string
	amountMinor int64
	entryType   string
	eventType   string
}

type savingsBalance struct {
	general int64
	savings int64
}

type SavingsServiceOption func(*SavingsService)

func WithSavingsBalanceLinkReader(linkReader SavingsBalanceLinkReader) SavingsServiceOption {
	return func(service *SavingsService) {
		service.linkReader = linkReader
	}
}

func NewSavingsService(entryReader SavingsEntryReader, eventRepo SavingsEventRepository, opts ...SavingsServiceOption) (*SavingsService, error) {
	if entryReader == nil {
		return nil, fmt.Errorf("savings service: entry reader is required")
	}
	if eventRepo == nil {
		return nil, fmt.Errorf("savings service: event repo is required")
	}

	service := &SavingsService{
		entryReader: entryReader,
		eventRepo:   eventRepo,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service, nil
}

func (s *SavingsService) AddTransfer(ctx context.Context, input SavingsAddInput) (domain.SavingsEvent, error) {
	return s.addEvent(ctx, domain.SavingsEventTypeTransferToSavings, input)
}

func (s *SavingsService) AddEntry(ctx context.Context, input SavingsAddInput) (domain.SavingsEvent, error) {
	return s.addEvent(ctx, domain.SavingsEventTypeIndependentAdd, input)
}

func (s *SavingsService) Show(ctx context.Context, req SavingsShowRequest) (domain.SavingsBalanceViews, error) {
	fromUTC, err := normalizeSavingsRangeBoundary(req.RangeFromUTC, false)
	if err != nil {
		return domain.SavingsBalanceViews{}, err
	}
	toUTC, err := normalizeSavingsRangeBoundary(req.RangeToUTC, true)
	if err != nil {
		return domain.SavingsBalanceViews{}, err
	}
	if err := domain.ValidateDateRange(fromUTC, toUTC); err != nil {
		return domain.SavingsBalanceViews{}, err
	}

	includeLifetime := req.IncludeLifetime
	includeRange := req.IncludeRange
	if !includeLifetime && !includeRange {
		includeLifetime = true
		includeRange = true
	}

	result := domain.SavingsBalanceViews{}
	if includeLifetime {
		view, err := s.computeView(ctx, domain.EntryListFilter{}, domain.SavingsEventListFilter{})
		if err != nil {
			return domain.SavingsBalanceViews{}, err
		}
		result.Lifetime = &view
	}

	if includeRange {
		view, err := s.computeView(
			ctx,
			domain.EntryListFilter{
				DateFromUTC: fromUTC,
				DateToUTC:   toUTC,
			},
			domain.SavingsEventListFilter{
				DateFromUTC: fromUTC,
				DateToUTC:   toUTC,
			},
		)
		if err != nil {
			return domain.SavingsBalanceViews{}, err
		}
		result.Range = &view
	}

	return result, nil
}

func (s *SavingsService) addEvent(ctx context.Context, eventType string, input SavingsAddInput) (domain.SavingsEvent, error) {
	normalizedEventType, err := domain.NormalizeSavingsEventType(eventType)
	if err != nil {
		return domain.SavingsEvent{}, err
	}
	if err := domain.ValidateAmountMinor(input.AmountMinor); err != nil {
		return domain.SavingsEvent{}, err
	}
	normalizedCurrency, err := domain.NormalizeCurrencyCode(input.CurrencyCode)
	if err != nil {
		return domain.SavingsEvent{}, err
	}
	normalizedDate, err := domain.NormalizeTransactionDateUTC(input.EventDateUTC)
	if err != nil {
		return domain.SavingsEvent{}, err
	}
	if err := domain.ValidateOptionalBankAccountID(input.SourceBankAccountID); err != nil {
		return domain.SavingsEvent{}, err
	}
	if err := domain.ValidateOptionalBankAccountID(input.DestinationBankAccountID); err != nil {
		return domain.SavingsEvent{}, err
	}

	sourceBankAccountID, destinationBankAccountID, err := s.resolveSavingsEventAccounts(ctx, normalizedEventType, input)
	if err != nil {
		return domain.SavingsEvent{}, err
	}

	return s.eventRepo.AddEvent(ctx, domain.SavingsEventAddInput{
		EventType:                normalizedEventType,
		AmountMinor:              input.AmountMinor,
		CurrencyCode:             normalizedCurrency,
		EventDateUTC:             normalizedDate,
		SourceBankAccountID:      sourceBankAccountID,
		DestinationBankAccountID: destinationBankAccountID,
		Note:                     strings.TrimSpace(input.Note),
	})
}

func (s *SavingsService) computeView(ctx context.Context, entryFilter domain.EntryListFilter, eventFilter domain.SavingsEventListFilter) (domain.SavingsBalanceView, error) {
	entries, err := s.entryReader.List(ctx, entryFilter)
	if err != nil {
		return domain.SavingsBalanceView{}, err
	}

	eventType, err := domain.NormalizeOptionalSavingsEventType(eventFilter.EventType)
	if err != nil {
		return domain.SavingsBalanceView{}, err
	}
	eventFilter.EventType = eventType

	events, err := s.eventRepo.ListEvents(ctx, eventFilter)
	if err != nil {
		return domain.SavingsBalanceView{}, err
	}

	items, err := buildSavingsReplay(entries, events)
	if err != nil {
		return domain.SavingsBalanceView{}, err
	}

	balances := map[string]savingsBalance{}
	for _, item := range items {
		state := balances[item.currency]
		switch item.kind {
		case savingsReplayKindEntry:
			switch item.entryType {
			case domain.EntryTypeIncome:
				state.general += item.amountMinor
			case domain.EntryTypeExpense:
				applySavingsExpense(&state, item.amountMinor)
			}
		case savingsReplayKindEvent:
			switch item.eventType {
			case domain.SavingsEventTypeTransferToSavings:
				state.general -= item.amountMinor
				state.savings += item.amountMinor
			case domain.SavingsEventTypeIndependentAdd:
				state.savings += item.amountMinor
			default:
				return domain.SavingsBalanceView{}, domain.ErrInvalidSavingsEventType
			}
		}
		balances[item.currency] = state
	}

	currencies := make([]string, 0, len(balances))
	for currency := range balances {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)

	rows := make([]domain.SavingsCurrencyBalance, 0, len(currencies))
	for _, currency := range currencies {
		state := balances[currency]
		rows = append(rows, domain.SavingsCurrencyBalance{
			CurrencyCode:        currency,
			GeneralBalanceMinor: state.general,
			SavingsBalanceMinor: state.savings,
			TotalBalanceMinor:   state.general + state.savings,
		})
	}

	return domain.SavingsBalanceView{ByCurrency: rows}, nil
}

func buildSavingsReplay(entries []domain.Entry, events []domain.SavingsEvent) ([]savingsReplayItem, error) {
	items := make([]savingsReplayItem, 0, len(entries)+len(events))

	for _, entry := range entries {
		dateValue, err := parseSavingsReplayDate(entry.TransactionDateUTC)
		if err != nil {
			return nil, err
		}
		items = append(items, savingsReplayItem{
			kind:        savingsReplayKindEntry,
			id:          entry.ID,
			date:        dateValue,
			currency:    entry.CurrencyCode,
			amountMinor: entry.AmountMinor,
			entryType:   entry.Type,
		})
	}

	for _, event := range events {
		dateValue, err := parseSavingsReplayDate(event.EventDateUTC)
		if err != nil {
			return nil, err
		}
		items = append(items, savingsReplayItem{
			kind:        savingsReplayKindEvent,
			id:          event.ID,
			date:        dateValue,
			currency:    event.CurrencyCode,
			amountMinor: event.AmountMinor,
			eventType:   event.EventType,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if !items[i].date.Equal(items[j].date) {
			return items[i].date.Before(items[j].date)
		}
		if items[i].id != items[j].id {
			return items[i].id < items[j].id
		}
		return items[i].kind < items[j].kind
	})

	return items, nil
}

func applySavingsExpense(state *savingsBalance, amountMinor int64) {
	if state == nil || amountMinor <= 0 {
		return
	}
	if state.general >= amountMinor {
		state.general -= amountMinor
		return
	}

	remaining := amountMinor - state.general
	state.general = 0

	if state.savings >= remaining {
		state.savings -= remaining
		return
	}

	remaining -= state.savings
	state.savings = 0
	state.general -= remaining
}

func normalizeSavingsRangeBoundary(raw string, endOfDay bool) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UTC().Format(time.RFC3339Nano), nil
		}
	}

	dateOnly, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return "", domain.ErrInvalidTransactionDate
	}

	if endOfDay {
		dateOnly = dateOnly.Add(24*time.Hour - time.Nanosecond)
	}

	return dateOnly.UTC().Format(time.RFC3339Nano), nil
}

func parseSavingsReplayDate(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, domain.ErrInvalidTransactionDate
}

func (s *SavingsService) resolveSavingsEventAccounts(ctx context.Context, eventType string, input SavingsAddInput) (*int64, *int64, error) {
	source := copyInt64Ptr(input.SourceBankAccountID)
	destination := copyInt64Ptr(input.DestinationBankAccountID)

	if s.linkReader == nil {
		return source, destination, nil
	}

	links, err := s.linkReader.ListBalanceLinks(ctx)
	if err != nil {
		return nil, nil, err
	}

	generalAccountID := linkedAccountIDByTarget(links, domain.BalanceLinkTargetGeneral)
	savingsAccountID := linkedAccountIDByTarget(links, domain.BalanceLinkTargetSavings)

	switch eventType {
	case domain.SavingsEventTypeTransferToSavings:
		if source == nil {
			source = copyInt64Ptr(generalAccountID)
		}
		if destination == nil {
			destination = copyInt64Ptr(savingsAccountID)
		}
	case domain.SavingsEventTypeIndependentAdd:
		if destination == nil {
			destination = copyInt64Ptr(savingsAccountID)
		}
	}

	return source, destination, nil
}

func copyInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
