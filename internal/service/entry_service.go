package service

import (
	"context"
	"fmt"
	"strings"

	"budgetto/internal/domain"
)

type EntryRepository interface {
	Add(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error)
	List(ctx context.Context, filter domain.EntryListFilter) ([]domain.Entry, error)
	Delete(ctx context.Context, id int64) (domain.EntryDeleteResult, error)
}

type EntryService struct {
	repo EntryRepository
}

func NewEntryService(repo EntryRepository) (*EntryService, error) {
	if repo == nil {
		return nil, fmt.Errorf("entry service: repo is required")
	}
	return &EntryService{repo: repo}, nil
}

func (s *EntryService) Add(ctx context.Context, input domain.EntryAddInput) (domain.Entry, error) {
	normalizedType, err := domain.NormalizeEntryType(input.Type)
	if err != nil {
		return domain.Entry{}, err
	}
	if err := domain.ValidateAmountMinor(input.AmountMinor); err != nil {
		return domain.Entry{}, err
	}
	normalizedCurrency, err := domain.NormalizeCurrencyCode(input.CurrencyCode)
	if err != nil {
		return domain.Entry{}, err
	}
	normalizedDate, err := domain.NormalizeTransactionDateUTC(input.TransactionDateUTC)
	if err != nil {
		return domain.Entry{}, err
	}
	if err := domain.ValidateOptionalCategoryID(input.CategoryID); err != nil {
		return domain.Entry{}, err
	}
	normalizedLabelIDs, err := domain.NormalizeLabelIDs(input.LabelIDs)
	if err != nil {
		return domain.Entry{}, err
	}

	return s.repo.Add(ctx, domain.EntryAddInput{
		Type:               normalizedType,
		AmountMinor:        input.AmountMinor,
		CurrencyCode:       normalizedCurrency,
		TransactionDateUTC: normalizedDate,
		CategoryID:         input.CategoryID,
		LabelIDs:           normalizedLabelIDs,
		Note:               strings.TrimSpace(input.Note),
	})
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

	entries, err := s.repo.List(ctx, normalizedFilter)
	if err != nil {
		return nil, err
	}

	if len(normalizedLabelIDs) == 0 {
		return entries, nil
	}

	return filterEntriesByLabelMode(entries, normalizedLabelIDs, normalizedLabelMode), nil
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
