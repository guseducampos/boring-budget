package service

import (
	"context"

	"budgetto/internal/domain"
)

type CategoryRepository interface {
	Add(ctx context.Context, name string) (domain.Category, error)
	List(ctx context.Context) ([]domain.Category, error)
	Rename(ctx context.Context, id int64, newName string) (domain.Category, error)
	SoftDelete(ctx context.Context, id int64) (domain.CategoryDeleteResult, error)
}

type CategoryService struct {
	repo CategoryRepository
}

func NewCategoryService(repo CategoryRepository) *CategoryService {
	return &CategoryService{repo: repo}
}

func (s *CategoryService) Add(ctx context.Context, name string) (domain.Category, error) {
	normalized, err := domain.NormalizeCategoryName(name)
	if err != nil {
		return domain.Category{}, err
	}
	return s.repo.Add(ctx, normalized)
}

func (s *CategoryService) List(ctx context.Context) ([]domain.Category, error) {
	return s.repo.List(ctx)
}

func (s *CategoryService) Rename(ctx context.Context, id int64, newName string) (domain.Category, error) {
	if id <= 0 {
		return domain.Category{}, domain.ErrInvalidCategoryID
	}

	normalized, err := domain.NormalizeCategoryName(newName)
	if err != nil {
		return domain.Category{}, err
	}

	return s.repo.Rename(ctx, id, normalized)
}

func (s *CategoryService) Delete(ctx context.Context, id int64) (domain.CategoryDeleteResult, error) {
	if id <= 0 {
		return domain.CategoryDeleteResult{}, domain.ErrInvalidCategoryID
	}
	return s.repo.SoftDelete(ctx, id)
}
