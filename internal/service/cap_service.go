package service

import (
	"context"
	"fmt"

	"budgetto/internal/domain"
)

type CapRepository interface {
	Set(ctx context.Context, input domain.CapSetInput) (domain.MonthlyCap, domain.MonthlyCapChange, error)
	GetByMonth(ctx context.Context, monthKey string) (domain.MonthlyCap, error)
	ListChangesByMonth(ctx context.Context, monthKey string) ([]domain.MonthlyCapChange, error)
	GetExpenseTotalByMonthAndCurrency(ctx context.Context, monthKey, currencyCode string) (int64, error)
}

type CapService struct {
	repo CapRepository
}

func NewCapService(repo CapRepository) (*CapService, error) {
	if repo == nil {
		return nil, fmt.Errorf("cap service: repo is required")
	}
	return &CapService{repo: repo}, nil
}

func (s *CapService) Set(ctx context.Context, input domain.CapSetInput) (domain.MonthlyCap, domain.MonthlyCapChange, error) {
	normalized, err := domain.NormalizeCapSetInput(input)
	if err != nil {
		return domain.MonthlyCap{}, domain.MonthlyCapChange{}, err
	}
	return s.repo.Set(ctx, normalized)
}

func (s *CapService) Show(ctx context.Context, monthKey string) (domain.MonthlyCap, error) {
	normalizedMonth, err := domain.NormalizeMonthKey(monthKey)
	if err != nil {
		return domain.MonthlyCap{}, err
	}
	return s.repo.GetByMonth(ctx, normalizedMonth)
}

func (s *CapService) History(ctx context.Context, monthKey string) ([]domain.MonthlyCapChange, error) {
	normalizedMonth, err := domain.NormalizeMonthKey(monthKey)
	if err != nil {
		return nil, err
	}
	return s.repo.ListChangesByMonth(ctx, normalizedMonth)
}

func (s *CapService) ExpenseTotalByMonthAndCurrency(ctx context.Context, monthKey, currencyCode string) (int64, error) {
	normalizedMonth, err := domain.NormalizeMonthKey(monthKey)
	if err != nil {
		return 0, err
	}

	normalizedCurrency, err := domain.NormalizeCurrencyCode(currencyCode)
	if err != nil {
		return 0, err
	}

	return s.repo.GetExpenseTotalByMonthAndCurrency(ctx, normalizedMonth, normalizedCurrency)
}
