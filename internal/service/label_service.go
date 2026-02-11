package service

import (
	"context"
	"fmt"

	"budgetto/internal/domain"
)

type LabelRepository interface {
	Add(ctx context.Context, name string) (domain.Label, error)
	List(ctx context.Context) ([]domain.Label, error)
	Rename(ctx context.Context, id int64, newName string) (domain.Label, error)
	Delete(ctx context.Context, id int64) (domain.LabelDeleteResult, error)
}

type LabelService struct {
	repo LabelRepository
}

func NewLabelService(repo LabelRepository) (*LabelService, error) {
	if repo == nil {
		return nil, fmt.Errorf("label service: repo is required")
	}
	return &LabelService{repo: repo}, nil
}

func (s *LabelService) Add(ctx context.Context, name string) (domain.Label, error) {
	normalized, err := domain.NormalizeLabelName(name)
	if err != nil {
		return domain.Label{}, err
	}
	return s.repo.Add(ctx, normalized)
}

func (s *LabelService) List(ctx context.Context) ([]domain.Label, error) {
	return s.repo.List(ctx)
}

func (s *LabelService) Rename(ctx context.Context, id int64, newName string) (domain.Label, error) {
	if err := domain.ValidateLabelID(id); err != nil {
		return domain.Label{}, err
	}

	normalized, err := domain.NormalizeLabelName(newName)
	if err != nil {
		return domain.Label{}, err
	}

	return s.repo.Rename(ctx, id, normalized)
}

func (s *LabelService) Delete(ctx context.Context, id int64) (domain.LabelDeleteResult, error) {
	if err := domain.ValidateLabelID(id); err != nil {
		return domain.LabelDeleteResult{}, err
	}
	return s.repo.Delete(ctx, id)
}
