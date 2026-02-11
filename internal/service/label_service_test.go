package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"budgetto/internal/domain"
)

type labelRepoStub struct {
	addFn    func(ctx context.Context, name string) (domain.Label, error)
	listFn   func(ctx context.Context) ([]domain.Label, error)
	renameFn func(ctx context.Context, id int64, newName string) (domain.Label, error)
	deleteFn func(ctx context.Context, id int64) (domain.LabelDeleteResult, error)
}

func (s *labelRepoStub) Add(ctx context.Context, name string) (domain.Label, error) {
	return s.addFn(ctx, name)
}

func (s *labelRepoStub) List(ctx context.Context) ([]domain.Label, error) {
	return s.listFn(ctx)
}

func (s *labelRepoStub) Rename(ctx context.Context, id int64, newName string) (domain.Label, error) {
	return s.renameFn(ctx, id, newName)
}

func (s *labelRepoStub) Delete(ctx context.Context, id int64) (domain.LabelDeleteResult, error) {
	return s.deleteFn(ctx, id)
}

func TestNewLabelServiceRequiresRepo(t *testing.T) {
	t.Parallel()

	_, err := NewLabelService(nil)
	if err == nil {
		t.Fatalf("expected error for nil repo")
	}
}

func TestLabelServiceAddNormalizesName(t *testing.T) {
	t.Parallel()

	var receivedName string
	svc, err := NewLabelService(&labelRepoStub{
		addFn: func(ctx context.Context, name string) (domain.Label, error) {
			receivedName = name
			return domain.Label{ID: 1, Name: name, CreatedAtUTC: time.Now().UTC(), UpdatedAtUTC: time.Now().UTC()}, nil
		},
		listFn:   func(context.Context) ([]domain.Label, error) { return nil, nil },
		renameFn: func(context.Context, int64, string) (domain.Label, error) { return domain.Label{}, nil },
		deleteFn: func(context.Context, int64) (domain.LabelDeleteResult, error) { return domain.LabelDeleteResult{}, nil },
	})
	if err != nil {
		t.Fatalf("new label service: %v", err)
	}

	label, err := svc.Add(context.Background(), "  groceries  ")
	if err != nil {
		t.Fatalf("add label: %v", err)
	}

	if receivedName != "groceries" {
		t.Fatalf("expected normalized name to be passed to repo, got %q", receivedName)
	}
	if label.Name != "groceries" {
		t.Fatalf("expected label name to be groceries, got %q", label.Name)
	}
}

func TestLabelServiceAddRejectsEmptyName(t *testing.T) {
	t.Parallel()

	svc, err := NewLabelService(&labelRepoStub{
		addFn:    func(context.Context, string) (domain.Label, error) { return domain.Label{}, nil },
		listFn:   func(context.Context) ([]domain.Label, error) { return nil, nil },
		renameFn: func(context.Context, int64, string) (domain.Label, error) { return domain.Label{}, nil },
		deleteFn: func(context.Context, int64) (domain.LabelDeleteResult, error) { return domain.LabelDeleteResult{}, nil },
	})
	if err != nil {
		t.Fatalf("new label service: %v", err)
	}

	_, err = svc.Add(context.Background(), "   ")
	if !errors.Is(err, domain.ErrInvalidLabelName) {
		t.Fatalf("expected ErrInvalidLabelName, got %v", err)
	}
}

func TestLabelServiceRenameRejectsInvalidID(t *testing.T) {
	t.Parallel()

	called := false
	svc, err := NewLabelService(&labelRepoStub{
		addFn:  func(context.Context, string) (domain.Label, error) { return domain.Label{}, nil },
		listFn: func(context.Context) ([]domain.Label, error) { return nil, nil },
		renameFn: func(context.Context, int64, string) (domain.Label, error) {
			called = true
			return domain.Label{}, nil
		},
		deleteFn: func(context.Context, int64) (domain.LabelDeleteResult, error) { return domain.LabelDeleteResult{}, nil },
	})
	if err != nil {
		t.Fatalf("new label service: %v", err)
	}

	_, err = svc.Rename(context.Background(), 0, "food")
	if !errors.Is(err, domain.ErrInvalidLabelID) {
		t.Fatalf("expected ErrInvalidLabelID, got %v", err)
	}
	if called {
		t.Fatalf("expected repo rename not to be called")
	}
}

func TestLabelServiceDeletePropagatesRepoError(t *testing.T) {
	t.Parallel()

	svc, err := NewLabelService(&labelRepoStub{
		addFn:  func(context.Context, string) (domain.Label, error) { return domain.Label{}, nil },
		listFn: func(context.Context) ([]domain.Label, error) { return nil, nil },
		renameFn: func(context.Context, int64, string) (domain.Label, error) {
			return domain.Label{}, nil
		},
		deleteFn: func(context.Context, int64) (domain.LabelDeleteResult, error) {
			return domain.LabelDeleteResult{}, domain.ErrLabelNotFound
		},
	})
	if err != nil {
		t.Fatalf("new label service: %v", err)
	}

	_, err = svc.Delete(context.Background(), 42)
	if !errors.Is(err, domain.ErrLabelNotFound) {
		t.Fatalf("expected ErrLabelNotFound, got %v", err)
	}
}
