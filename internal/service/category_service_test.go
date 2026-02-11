package service

import (
	"context"
	"errors"
	"testing"

	"budgetto/internal/domain"
)

type categoryRepoStub struct {
	addFn        func(ctx context.Context, name string) (domain.Category, error)
	listFn       func(ctx context.Context) ([]domain.Category, error)
	renameFn     func(ctx context.Context, id int64, newName string) (domain.Category, error)
	softDeleteFn func(ctx context.Context, id int64) (domain.CategoryDeleteResult, error)
}

func (s categoryRepoStub) Add(ctx context.Context, name string) (domain.Category, error) {
	return s.addFn(ctx, name)
}

func (s categoryRepoStub) List(ctx context.Context) ([]domain.Category, error) {
	return s.listFn(ctx)
}

func (s categoryRepoStub) Rename(ctx context.Context, id int64, newName string) (domain.Category, error) {
	return s.renameFn(ctx, id, newName)
}

func (s categoryRepoStub) SoftDelete(ctx context.Context, id int64) (domain.CategoryDeleteResult, error) {
	return s.softDeleteFn(ctx, id)
}

func TestCategoryServiceAddTrimsName(t *testing.T) {
	t.Parallel()

	repo := categoryRepoStub{
		addFn: func(ctx context.Context, name string) (domain.Category, error) {
			if name != "Groceries" {
				t.Fatalf("expected normalized name Groceries, got %q", name)
			}
			return domain.Category{ID: 1, Name: name}, nil
		},
	}

	service := NewCategoryService(repo)
	category, err := service.Add(context.Background(), "  Groceries  ")
	if err != nil {
		t.Fatalf("add category: %v", err)
	}
	if category.Name != "Groceries" {
		t.Fatalf("expected returned category name Groceries, got %q", category.Name)
	}
}

func TestCategoryServiceRenameRejectsInvalidID(t *testing.T) {
	t.Parallel()

	service := NewCategoryService(categoryRepoStub{
		addFn: func(ctx context.Context, name string) (domain.Category, error) {
			return domain.Category{}, nil
		},
		listFn: func(ctx context.Context) ([]domain.Category, error) {
			return nil, nil
		},
		renameFn: func(ctx context.Context, id int64, newName string) (domain.Category, error) {
			return domain.Category{}, nil
		},
		softDeleteFn: func(ctx context.Context, id int64) (domain.CategoryDeleteResult, error) {
			return domain.CategoryDeleteResult{}, nil
		},
	})

	_, err := service.Rename(context.Background(), 0, "Food")
	if !errors.Is(err, domain.ErrInvalidCategoryID) {
		t.Fatalf("expected ErrInvalidCategoryID, got %v", err)
	}
}

func TestCategoryServiceDeleteRejectsInvalidID(t *testing.T) {
	t.Parallel()

	service := NewCategoryService(categoryRepoStub{
		addFn: func(ctx context.Context, name string) (domain.Category, error) {
			return domain.Category{}, nil
		},
		listFn: func(ctx context.Context) ([]domain.Category, error) {
			return nil, nil
		},
		renameFn: func(ctx context.Context, id int64, newName string) (domain.Category, error) {
			return domain.Category{}, nil
		},
		softDeleteFn: func(ctx context.Context, id int64) (domain.CategoryDeleteResult, error) {
			return domain.CategoryDeleteResult{}, nil
		},
	})

	_, err := service.Delete(context.Background(), -10)
	if !errors.Is(err, domain.ErrInvalidCategoryID) {
		t.Fatalf("expected ErrInvalidCategoryID, got %v", err)
	}
}

func TestCategoryServiceListReturnsRepoResult(t *testing.T) {
	t.Parallel()

	expected := []domain.Category{
		{ID: 1, Name: "Food"},
		{ID: 2, Name: "Rent"},
	}

	service := NewCategoryService(categoryRepoStub{
		addFn: func(ctx context.Context, name string) (domain.Category, error) {
			return domain.Category{}, nil
		},
		listFn: func(ctx context.Context) ([]domain.Category, error) {
			return expected, nil
		},
		renameFn: func(ctx context.Context, id int64, newName string) (domain.Category, error) {
			return domain.Category{}, nil
		},
		softDeleteFn: func(ctx context.Context, id int64) (domain.CategoryDeleteResult, error) {
			return domain.CategoryDeleteResult{}, nil
		},
	})

	categories, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("list categories: %v", err)
	}
	if len(categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(categories))
	}
	if categories[0].Name != "Food" || categories[1].Name != "Rent" {
		t.Fatalf("unexpected categories: %+v", categories)
	}
}
