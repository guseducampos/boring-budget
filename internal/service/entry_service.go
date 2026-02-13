package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"boring-budget/internal/domain"
	"boring-budget/internal/ports"
)

type EntryService struct {
	repo         EntryRepository
	capLookup    EntryCapLookup
	cardResolver EntryCardResolver
}

type EntryRepository = ports.EntryRepository
type EntryCapLookup = ports.EntryCapLookup
type EntryRepositoryTxBinder = ports.EntryRepositoryTxBinder
type EntryCapLookupTxBinder = ports.EntryCapLookupTxBinder

type EntryCreditLiabilitySyncer interface {
	SyncCreditLiabilityCharge(ctx context.Context, entryID int64) error
}

type EntryCardResolver interface {
	Resolve(ctx context.Context, selector domain.CardSelector) (domain.Card, error)
}

type EntryServiceOption func(*EntryService)

func WithEntryCapLookup(capLookup EntryCapLookup) EntryServiceOption {
	return func(service *EntryService) {
		service.capLookup = capLookup
	}
}

func WithEntryCardResolver(cardResolver EntryCardResolver) EntryServiceOption {
	return func(service *EntryService) {
		service.cardResolver = cardResolver
	}
}

type EntryAddResult struct {
	Entry    domain.Entry     `json:"entry"`
	Warnings []domain.Warning `json:"warnings"`
}

func NewEntryService(repo EntryRepository, opts ...EntryServiceOption) (*EntryService, error) {
	if repo == nil {
		return nil, fmt.Errorf("entry service: repo is required")
	}

	service := &EntryService{repo: repo}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}

	return service, nil
}

func (s *EntryService) Add(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error) {
	result, err := s.AddWithWarnings(ctx, input)
	if err != nil {
		return domain.Entry{}, err
	}
	return result.Entry, nil
}

func (s *EntryService) AddWithWarnings(ctx context.Context, input domain.EntryAddInput) (EntryAddResult, error) {
	normalizedType, err := domain.NormalizeEntryType(input.Type)
	if err != nil {
		return EntryAddResult{}, err
	}
	if err := domain.ValidateAmountMinor(input.AmountMinor); err != nil {
		return EntryAddResult{}, err
	}
	normalizedCurrency, err := domain.NormalizeCurrencyCode(input.CurrencyCode)
	if err != nil {
		return EntryAddResult{}, err
	}
	normalizedDate, err := domain.NormalizeTransactionDateUTC(input.TransactionDateUTC)
	if err != nil {
		return EntryAddResult{}, err
	}
	if err := domain.ValidateOptionalCategoryID(input.CategoryID); err != nil {
		return EntryAddResult{}, err
	}
	normalizedLabelIDs, err := domain.NormalizeLabelIDs(input.LabelIDs)
	if err != nil {
		return EntryAddResult{}, err
	}
	normalizedPaymentMethod, err := domain.NormalizePaymentMethod(input.PaymentMethod)
	if err != nil {
		return EntryAddResult{}, err
	}
	if err := domain.ValidateCardSelector(input.PaymentCardID, input.PaymentCardNickname, input.PaymentCardLookup); err != nil {
		return EntryAddResult{}, err
	}
	hasCardSelector := domain.HasCardSelector(input.PaymentCardID, input.PaymentCardNickname, input.PaymentCardLookup)

	if normalizedType != domain.EntryTypeExpense {
		if normalizedPaymentMethod != "" || hasCardSelector {
			return EntryAddResult{}, domain.ErrPaymentNotAllowed
		}
	} else {
		if normalizedPaymentMethod == "" {
			normalizedPaymentMethod = domain.PaymentMethodCash
		}
		if normalizedPaymentMethod == domain.PaymentMethodCash && hasCardSelector {
			return EntryAddResult{}, domain.ErrCardNotAllowed
		}
		if normalizedPaymentMethod == domain.PaymentMethodCard && !hasCardSelector {
			return EntryAddResult{}, domain.ErrCardRequired
		}
	}

	resolvedCardID, err := s.resolvePaymentCardID(ctx, input.PaymentCardID, input.PaymentCardNickname, input.PaymentCardLookup)
	if err != nil {
		return EntryAddResult{}, err
	}

	entry, err := s.repo.Add(ctx, domain.EntryAddInput{
		Type:               normalizedType,
		AmountMinor:        input.AmountMinor,
		CurrencyCode:       normalizedCurrency,
		TransactionDateUTC: normalizedDate,
		CategoryID:         input.CategoryID,
		LabelIDs:           normalizedLabelIDs,
		Note:               strings.TrimSpace(input.Note),
		PaymentMethod:      normalizedPaymentMethod,
		PaymentCardID:      resolvedCardID,
	})
	if err != nil {
		return EntryAddResult{}, err
	}
	if err := s.syncCreditLiability(ctx, entry); err != nil {
		return EntryAddResult{}, err
	}

	result := EntryAddResult{
		Entry:    entry,
		Warnings: []domain.Warning{},
	}

	if entry.Type != domain.EntryTypeExpense || s.capLookup == nil {
		return result, nil
	}

	monthKey, err := domain.MonthKeyFromDateTimeUTC(entry.TransactionDateUTC)
	if err != nil {
		return result, nil
	}

	capValue, err := s.capLookup.GetByMonth(ctx, monthKey)
	if err != nil {
		if errors.Is(err, domain.ErrCapNotFound) {
			return result, nil
		}
		return result, nil
	}

	if capValue.CurrencyCode != entry.CurrencyCode {
		return result, nil
	}

	totalSpend, err := s.capLookup.GetExpenseTotalByMonthAndCurrency(ctx, monthKey, entry.CurrencyCode)
	if err != nil {
		return result, nil
	}

	if totalSpend <= capValue.AmountMinor {
		return result, nil
	}

	result.Warnings = append(result.Warnings, domain.Warning{
		Code:    domain.WarningCodeCapExceeded,
		Message: domain.CapExceededWarningMessage,
		Details: domain.CapExceededWarningDetails{
			MonthKey: monthKey,
			CapAmount: domain.MoneyAmount{
				AmountMinor:  capValue.AmountMinor,
				CurrencyCode: capValue.CurrencyCode,
			},
			NewSpendTotal: domain.MoneyAmount{
				AmountMinor:  totalSpend,
				CurrencyCode: entry.CurrencyCode,
			},
			OverspendAmount: domain.MoneyAmount{
				AmountMinor:  totalSpend - capValue.AmountMinor,
				CurrencyCode: entry.CurrencyCode,
			},
		},
	})

	return result, nil
}

func (s *EntryService) List(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error) {
	normalizedFilter := domain.EntryListFilter{}

	if strings.TrimSpace(filter.Type) != "" {
		normalizedType, err := domain.NormalizeEntryType(filter.Type)
		if err != nil {
			return nil, err
		}
		normalizedFilter.Type = normalizedType
	}

	if err := domain.ValidateOptionalCategoryID(filter.CategoryID); err != nil {
		return nil, err
	}
	normalizedFilter.CategoryID = filter.CategoryID

	dateFromUTC, err := domain.NormalizeOptionalTransactionDateUTC(filter.DateFromUTC)
	if err != nil {
		return nil, err
	}
	dateToUTC, err := domain.NormalizeOptionalTransactionDateUTC(filter.DateToUTC)
	if err != nil {
		return nil, err
	}
	if err := domain.ValidateDateRange(dateFromUTC, dateToUTC); err != nil {
		return nil, err
	}
	normalizedFilter.DateFromUTC = dateFromUTC
	normalizedFilter.DateToUTC = dateToUTC
	normalizedFilter.NoteContains = strings.TrimSpace(filter.NoteContains)

	normalizedLabelIDs, err := domain.NormalizeLabelIDs(filter.LabelIDs)
	if err != nil {
		return nil, err
	}
	normalizedFilter.LabelIDs = normalizedLabelIDs

	normalizedLabelMode, err := domain.NormalizeLabelMode(filter.LabelMode)
	if err != nil {
		return nil, err
	}
	normalizedFilter.LabelMode = normalizedLabelMode
	normalizedPaymentMethod, err := domain.NormalizePaymentMethodFilter(filter.PaymentMethod)
	if err != nil {
		return nil, err
	}
	if err := domain.ValidateCardSelector(filter.PaymentCardID, filter.PaymentCardNickname, filter.PaymentCardLookup); err != nil {
		return nil, err
	}
	if normalizedPaymentMethod == domain.PaymentMethodCash && domain.HasCardSelector(filter.PaymentCardID, filter.PaymentCardNickname, filter.PaymentCardLookup) {
		return nil, domain.ErrCardNotAllowed
	}
	normalizedFilter.PaymentMethod = normalizedPaymentMethod
	normalizedFilter.PaymentCardID = filter.PaymentCardID
	normalizedFilter.PaymentCardNickname = strings.TrimSpace(filter.PaymentCardNickname)
	normalizedFilter.PaymentCardLookup = strings.TrimSpace(filter.PaymentCardLookup)

	entries, err := s.repo.List(ctx, normalizedFilter)
	if err != nil {
		return nil, err
	}

	if len(normalizedLabelIDs) == 0 {
		return entries, nil
	}

	return filterEntriesByLabelMode(entries, normalizedLabelIDs, normalizedLabelMode), nil
}

func (s *EntryService) Update(ctx context.Context, input domain.EntryUpdateInput) (domain.Entry, error) {
	result, err := s.UpdateWithWarnings(ctx, input)
	if err != nil {
		return domain.Entry{}, err
	}
	return result.Entry, nil
}

func (s *EntryService) UpdateWithWarnings(ctx context.Context, input domain.EntryUpdateInput) (EntryAddResult, error) {
	if err := domain.ValidateEntryID(input.ID); err != nil {
		return EntryAddResult{}, err
	}
	if !domain.HasEntryUpdateChanges(input) {
		return EntryAddResult{}, domain.ErrNoEntryUpdateFields
	}

	normalized := domain.EntryUpdateInput{ID: input.ID}

	if input.Type != nil {
		normalizedType, err := domain.NormalizeEntryType(*input.Type)
		if err != nil {
			return EntryAddResult{}, err
		}
		normalized.Type = &normalizedType
	}

	if input.AmountMinor != nil {
		if err := domain.ValidateAmountMinor(*input.AmountMinor); err != nil {
			return EntryAddResult{}, err
		}
		value := *input.AmountMinor
		normalized.AmountMinor = &value
	}

	if input.CurrencyCode != nil {
		normalizedCurrency, err := domain.NormalizeCurrencyCode(*input.CurrencyCode)
		if err != nil {
			return EntryAddResult{}, err
		}
		normalized.CurrencyCode = &normalizedCurrency
	}

	if input.TransactionDateUTC != nil {
		normalizedDate, err := domain.NormalizeTransactionDateUTC(*input.TransactionDateUTC)
		if err != nil {
			return EntryAddResult{}, err
		}
		normalized.TransactionDateUTC = &normalizedDate
	}

	if input.SetCategory {
		normalized.SetCategory = true
		if input.CategoryID != nil {
			if err := domain.ValidateOptionalCategoryID(input.CategoryID); err != nil {
				return EntryAddResult{}, err
			}
			categoryID := *input.CategoryID
			normalized.CategoryID = &categoryID
		}
	}

	if input.SetLabelIDs {
		normalizedLabelIDs, err := domain.NormalizeLabelIDs(input.LabelIDs)
		if err != nil {
			return EntryAddResult{}, err
		}
		normalized.SetLabelIDs = true
		normalized.LabelIDs = normalizedLabelIDs
	}

	if input.SetNote {
		normalized.SetNote = true
		if input.Note != nil {
			value := strings.TrimSpace(*input.Note)
			normalized.Note = &value
		}
	}

	if input.SetPaymentMethod {
		normalized.SetPaymentMethod = true
		if input.PaymentMethod != nil {
			paymentMethod, err := domain.NormalizePaymentMethod(*input.PaymentMethod)
			if err != nil {
				return EntryAddResult{}, err
			}
			if paymentMethod == "" {
				return EntryAddResult{}, domain.ErrInvalidPaymentMethod
			}
			normalized.PaymentMethod = &paymentMethod
		}
	}

	if input.SetPaymentCard {
		normalized.SetPaymentCard = true
		if err := domain.ValidateCardSelector(input.PaymentCardID, derefString(input.PaymentCardNickname), derefString(input.PaymentCardLookup)); err != nil {
			return EntryAddResult{}, err
		}
		resolvedCardID, err := s.resolvePaymentCardID(ctx, input.PaymentCardID, derefString(input.PaymentCardNickname), derefString(input.PaymentCardLookup))
		if err != nil {
			return EntryAddResult{}, err
		}
		normalized.PaymentCardID = resolvedCardID
	}

	if normalized.SetPaymentCard && normalized.PaymentCardID != nil && !normalized.SetPaymentMethod {
		method := domain.PaymentMethodCard
		normalized.SetPaymentMethod = true
		normalized.PaymentMethod = &method
	}

	if normalized.SetPaymentMethod && normalized.PaymentMethod != nil && *normalized.PaymentMethod == domain.PaymentMethodCash {
		normalized.SetPaymentCard = true
		normalized.PaymentCardID = nil
	}

	entry, err := s.repo.Update(ctx, normalized)
	if err != nil {
		return EntryAddResult{}, err
	}
	if err := s.syncCreditLiability(ctx, entry); err != nil {
		return EntryAddResult{}, err
	}

	result := EntryAddResult{
		Entry:    entry,
		Warnings: []domain.Warning{},
	}

	if entry.Type != domain.EntryTypeExpense || s.capLookup == nil {
		return result, nil
	}

	monthKey, err := domain.MonthKeyFromDateTimeUTC(entry.TransactionDateUTC)
	if err != nil {
		return result, nil
	}

	capValue, err := s.capLookup.GetByMonth(ctx, monthKey)
	if err != nil {
		if errors.Is(err, domain.ErrCapNotFound) {
			return result, nil
		}
		return result, nil
	}

	if capValue.CurrencyCode != entry.CurrencyCode {
		return result, nil
	}

	totalSpend, err := s.capLookup.GetExpenseTotalByMonthAndCurrency(ctx, monthKey, entry.CurrencyCode)
	if err != nil {
		return result, nil
	}

	if totalSpend <= capValue.AmountMinor {
		return result, nil
	}

	result.Warnings = append(result.Warnings, domain.Warning{
		Code:    domain.WarningCodeCapExceeded,
		Message: domain.CapExceededWarningMessage,
		Details: domain.CapExceededWarningDetails{
			MonthKey: monthKey,
			CapAmount: domain.MoneyAmount{
				AmountMinor:  capValue.AmountMinor,
				CurrencyCode: capValue.CurrencyCode,
			},
			NewSpendTotal: domain.MoneyAmount{
				AmountMinor:  totalSpend,
				CurrencyCode: entry.CurrencyCode,
			},
			OverspendAmount: domain.MoneyAmount{
				AmountMinor:  totalSpend - capValue.AmountMinor,
				CurrencyCode: entry.CurrencyCode,
			},
		},
	})

	return result, nil
}

func (s *EntryService) Delete(ctx context.Context, id int64) (domain.EntryDeleteResult, error) {
	if err := domain.ValidateEntryID(id); err != nil {
		return domain.EntryDeleteResult{}, err
	}
	return s.repo.Delete(ctx, id)
}

func filterEntriesByLabelMode(entries []domain.Entry, labelIDs []int64, mode string) []domain.Entry {
	if len(entries) == 0 || len(labelIDs) == 0 {
		return entries
	}

	requestedSet := make(map[int64]struct{}, len(labelIDs))
	for _, labelID := range labelIDs {
		requestedSet[labelID] = struct{}{}
	}

	filtered := make([]domain.Entry, 0, len(entries))
	for _, entry := range entries {
		entrySet := make(map[int64]struct{}, len(entry.LabelIDs))
		for _, labelID := range entry.LabelIDs {
			entrySet[labelID] = struct{}{}
		}

		matchesAny := false
		missingAny := false
		for labelID := range requestedSet {
			_, has := entrySet[labelID]
			if has {
				matchesAny = true
			} else {
				missingAny = true
			}
		}

		keep := false
		switch mode {
		case domain.LabelFilterModeAny:
			keep = matchesAny
		case domain.LabelFilterModeAll:
			keep = !missingAny
		case domain.LabelFilterModeNone:
			keep = !matchesAny
		default:
			keep = matchesAny
		}

		if keep {
			filtered = append(filtered, entry)
		}
	}

	return filtered
}

func (s *EntryService) syncCreditLiability(ctx context.Context, entry domain.Entry) error {
	if strings.TrimSpace(entry.Type) != domain.EntryTypeExpense {
		return nil
	}

	syncer, ok := s.repo.(EntryCreditLiabilitySyncer)
	if !ok || syncer == nil {
		return nil
	}

	return syncer.SyncCreditLiabilityCharge(ctx, entry.ID)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *EntryService) resolvePaymentCardID(ctx context.Context, cardID *int64, cardNickname, cardLookup string) (*int64, error) {
	if cardID != nil {
		value := *cardID
		return &value, nil
	}

	trimmedNickname := strings.TrimSpace(cardNickname)
	trimmedLookup := strings.TrimSpace(cardLookup)
	if trimmedNickname == "" && trimmedLookup == "" {
		return nil, nil
	}

	if s.cardResolver == nil {
		return nil, domain.ErrCardNotFound
	}

	card, err := s.cardResolver.Resolve(ctx, domain.CardSelector{
		Nickname: trimmedNickname,
		Lookup:   trimmedLookup,
	})
	if err != nil {
		return nil, err
	}

	value := card.ID
	return &value, nil
}
