package service

import (
	"context"
	"errors"
	"testing"

	"boring-budget/internal/domain"
)

type bankAccountRepoStub struct {
	addFn    func(ctx context.Context, input domain.BankAccountAddInput) (domain.BankAccount, error)
	getByID  func(ctx context.Context, id int64, includeDeleted bool) (domain.BankAccount, error)
	listFn   func(ctx context.Context, filter domain.BankAccountListFilter) ([]domain.BankAccount, error)
	searchFn func(ctx context.Context, lookup string, limit int32) ([]domain.BankAccount, error)
	updateFn func(ctx context.Context, input domain.BankAccountUpdateInput) (domain.BankAccount, error)
	deleteFn func(ctx context.Context, id int64) (domain.BankAccountDeleteResult, error)
	linkFn   func(ctx context.Context, target string, bankAccountID *int64) error
	linksFn  func(ctx context.Context) ([]domain.BalanceAccountLink, error)
}

func (s bankAccountRepoStub) Add(ctx context.Context, input domain.BankAccountAddInput) (domain.BankAccount, error) {
	if s.addFn == nil {
		return domain.BankAccount{}, nil
	}
	return s.addFn(ctx, input)
}

func (s bankAccountRepoStub) GetByID(ctx context.Context, id int64, includeDeleted bool) (domain.BankAccount, error) {
	if s.getByID == nil {
		return domain.BankAccount{}, nil
	}
	return s.getByID(ctx, id, includeDeleted)
}

func (s bankAccountRepoStub) List(ctx context.Context, filter domain.BankAccountListFilter) ([]domain.BankAccount, error) {
	if s.listFn == nil {
		return []domain.BankAccount{}, nil
	}
	return s.listFn(ctx, filter)
}

func (s bankAccountRepoStub) Search(ctx context.Context, lookup string, limit int32) ([]domain.BankAccount, error) {
	if s.searchFn == nil {
		return []domain.BankAccount{}, nil
	}
	return s.searchFn(ctx, lookup, limit)
}

func (s bankAccountRepoStub) Update(ctx context.Context, input domain.BankAccountUpdateInput) (domain.BankAccount, error) {
	if s.updateFn == nil {
		return domain.BankAccount{}, nil
	}
	return s.updateFn(ctx, input)
}

func (s bankAccountRepoStub) Delete(ctx context.Context, id int64) (domain.BankAccountDeleteResult, error) {
	if s.deleteFn == nil {
		return domain.BankAccountDeleteResult{}, nil
	}
	return s.deleteFn(ctx, id)
}

func (s bankAccountRepoStub) UpsertBalanceLink(ctx context.Context, target string, bankAccountID *int64) error {
	if s.linkFn == nil {
		return nil
	}
	return s.linkFn(ctx, target, bankAccountID)
}

func (s bankAccountRepoStub) ListBalanceLinks(ctx context.Context) ([]domain.BalanceAccountLink, error) {
	if s.linksFn == nil {
		return []domain.BalanceAccountLink{}, nil
	}
	return s.linksFn(ctx)
}

func TestBankAccountServiceAddNormalizesInput(t *testing.T) {
	t.Parallel()

	svc, err := NewBankAccountService(bankAccountRepoStub{
		addFn: func(ctx context.Context, input domain.BankAccountAddInput) (domain.BankAccount, error) {
			if input.Alias != "Main Checking" {
				t.Fatalf("expected normalized alias, got %q", input.Alias)
			}
			if input.Last4 != "1234" {
				t.Fatalf("expected normalized last4, got %q", input.Last4)
			}
			return domain.BankAccount{ID: 10, Alias: input.Alias, Last4: input.Last4}, nil
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, err := svc.Add(context.Background(), domain.BankAccountAddInput{Alias: "  Main Checking  ", Last4: " 1234 "})
	if err != nil {
		t.Fatalf("add bank account: %v", err)
	}
	if account.Alias != "Main Checking" {
		t.Fatalf("expected alias Main Checking, got %q", account.Alias)
	}
}

func TestBankAccountServiceListLookupAndIncludeDeleted(t *testing.T) {
	t.Parallel()

	svc, err := NewBankAccountService(bankAccountRepoStub{
		listFn: func(ctx context.Context, filter domain.BankAccountListFilter) ([]domain.BankAccount, error) {
			if !filter.IncludeDeleted {
				t.Fatalf("expected include_deleted=true when filtering deleted by lookup")
			}
			deletedAt := "2026-02-01T00:00:00Z"
			return []domain.BankAccount{
				{ID: 3, Alias: "zeta", Last4: "9999"},
				{ID: 2, Alias: "Alpha Savings", Last4: "1111", DeletedAtUTC: &deletedAt},
				{ID: 1, Alias: "alpha checking", Last4: "1234"},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	accounts, err := svc.List(context.Background(), domain.BankAccountListFilter{
		Lookup:         "alpha",
		IncludeDeleted: true,
	})
	if err != nil {
		t.Fatalf("list bank accounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}
	if accounts[0].ID != 1 || accounts[1].ID != 2 {
		t.Fatalf("expected deterministic order by alias then id, got %+v", accounts)
	}
}

func TestBankAccountServiceListLookupUsesSearchForActiveOnly(t *testing.T) {
	t.Parallel()

	svc, err := NewBankAccountService(bankAccountRepoStub{
		searchFn: func(ctx context.Context, lookup string, limit int32) ([]domain.BankAccount, error) {
			if lookup != "123" {
				t.Fatalf("expected lookup 123, got %q", lookup)
			}
			if limit != bankAccountLookupSearchLimit {
				t.Fatalf("unexpected lookup limit: %d", limit)
			}
			return []domain.BankAccount{
				{ID: 4, Alias: "Beta", Last4: "0123"},
				{ID: 3, Alias: "alpha", Last4: "1234"},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	accounts, err := svc.List(context.Background(), domain.BankAccountListFilter{Lookup: "123"})
	if err != nil {
		t.Fatalf("list bank accounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}
	if accounts[0].Alias != "alpha" || accounts[1].Alias != "Beta" {
		t.Fatalf("expected sorted aliases [alpha, Beta], got %+v", accounts)
	}
}

func TestBankAccountServiceUpdateRejectsNoChanges(t *testing.T) {
	t.Parallel()

	svc, err := NewBankAccountService(bankAccountRepoStub{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.Update(context.Background(), domain.BankAccountUpdateInput{ID: 10})
	if !errors.Is(err, domain.ErrNoBankAccountUpdateFields) {
		t.Fatalf("expected ErrNoBankAccountUpdateFields, got %v", err)
	}
}

func TestBankAccountServiceUpdateNormalizesValues(t *testing.T) {
	t.Parallel()

	svc, err := NewBankAccountService(bankAccountRepoStub{
		updateFn: func(ctx context.Context, input domain.BankAccountUpdateInput) (domain.BankAccount, error) {
			if input.Alias == nil || *input.Alias != "Emergency" {
				t.Fatalf("expected normalized alias Emergency, got %+v", input.Alias)
			}
			if input.Last4 == nil || *input.Last4 != "4444" {
				t.Fatalf("expected normalized last4 4444, got %+v", input.Last4)
			}
			return domain.BankAccount{ID: input.ID, Alias: *input.Alias, Last4: *input.Last4}, nil
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	alias := "  Emergency "
	last4 := " 4444 "
	updated, err := svc.Update(context.Background(), domain.BankAccountUpdateInput{
		ID:    9,
		Alias: &alias,
		Last4: &last4,
	})
	if err != nil {
		t.Fatalf("update bank account: %v", err)
	}
	if updated.ID != 9 || updated.Alias != "Emergency" || updated.Last4 != "4444" {
		t.Fatalf("unexpected update result: %+v", updated)
	}
}

func TestBankAccountServiceDeleteRejectsInvalidID(t *testing.T) {
	t.Parallel()

	svc, err := NewBankAccountService(bankAccountRepoStub{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.Delete(context.Background(), 0)
	if !errors.Is(err, domain.ErrInvalidBankAccountID) {
		t.Fatalf("expected ErrInvalidBankAccountID, got %v", err)
	}
}
