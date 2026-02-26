package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"boring-budget/internal/domain"
)

const bankAccountLookupSearchLimit = int32(25)

type BankAccountRepository interface {
	Add(ctx context.Context, input domain.BankAccountAddInput) (domain.BankAccount, error)
	GetByID(ctx context.Context, id int64, includeDeleted bool) (domain.BankAccount, error)
	List(ctx context.Context, filter domain.BankAccountListFilter) ([]domain.BankAccount, error)
	Search(ctx context.Context, lookup string, limit int32) ([]domain.BankAccount, error)
	Update(ctx context.Context, input domain.BankAccountUpdateInput) (domain.BankAccount, error)
	Delete(ctx context.Context, id int64) (domain.BankAccountDeleteResult, error)
	UpsertBalanceLink(ctx context.Context, target string, bankAccountID *int64) error
	ListBalanceLinks(ctx context.Context) ([]domain.BalanceAccountLink, error)
}

type BankAccountService struct {
	repo BankAccountRepository
}

func NewBankAccountService(repo BankAccountRepository) (*BankAccountService, error) {
	if repo == nil {
		return nil, fmt.Errorf("bank account service: repo is required")
	}
	return &BankAccountService{repo: repo}, nil
}

func (s *BankAccountService) Add(ctx context.Context, input domain.BankAccountAddInput) (domain.BankAccount, error) {
	normalized, err := domain.NormalizeBankAccountAddInput(input)
	if err != nil {
		return domain.BankAccount{}, err
	}

	return s.repo.Add(ctx, normalized)
}

func (s *BankAccountService) List(ctx context.Context, filter domain.BankAccountListFilter) ([]domain.BankAccount, error) {
	lookup := strings.TrimSpace(filter.Lookup)
	if lookup == "" {
		accounts, err := s.repo.List(ctx, domain.BankAccountListFilter{IncludeDeleted: filter.IncludeDeleted})
		if err != nil {
			return nil, err
		}
		sortBankAccountsDeterministic(accounts)
		return accounts, nil
	}

	normalizedLookup, err := domain.NormalizeBankAccountLookup(lookup)
	if err != nil {
		return nil, err
	}

	if !filter.IncludeDeleted {
		accounts, err := s.repo.Search(ctx, normalizedLookup, bankAccountLookupSearchLimit)
		if err != nil {
			return nil, err
		}
		sortBankAccountsDeterministic(accounts)
		return accounts, nil
	}

	accounts, err := s.repo.List(ctx, domain.BankAccountListFilter{IncludeDeleted: true})
	if err != nil {
		return nil, err
	}

	filtered := make([]domain.BankAccount, 0, len(accounts))
	for _, account := range accounts {
		if matchesBankAccountLookup(account, normalizedLookup) {
			filtered = append(filtered, account)
		}
	}

	sortBankAccountsDeterministic(filtered)
	return filtered, nil
}

func (s *BankAccountService) Update(ctx context.Context, input domain.BankAccountUpdateInput) (domain.BankAccount, error) {
	if err := domain.ValidateBankAccountID(input.ID); err != nil {
		return domain.BankAccount{}, err
	}
	if input.Alias == nil && input.Last4 == nil {
		return domain.BankAccount{}, domain.ErrNoBankAccountUpdateFields
	}

	normalized := domain.BankAccountUpdateInput{ID: input.ID}
	if input.Alias != nil {
		value, err := domain.NormalizeBankAccountAlias(*input.Alias)
		if err != nil {
			return domain.BankAccount{}, err
		}
		normalized.Alias = &value
	}
	if input.Last4 != nil {
		value, err := domain.NormalizeBankAccountLast4(*input.Last4)
		if err != nil {
			return domain.BankAccount{}, err
		}
		normalized.Last4 = &value
	}

	return s.repo.Update(ctx, normalized)
}

func (s *BankAccountService) Delete(ctx context.Context, id int64) (domain.BankAccountDeleteResult, error) {
	if err := domain.ValidateBankAccountID(id); err != nil {
		return domain.BankAccountDeleteResult{}, err
	}
	return s.repo.Delete(ctx, id)
}

func (s *BankAccountService) SetBalanceLink(ctx context.Context, target string, bankAccountID *int64) (domain.BalanceAccountLink, error) {
	normalizedTarget, err := domain.NormalizeBalanceLinkTarget(target)
	if err != nil {
		return domain.BalanceAccountLink{}, err
	}

	var normalizedID *int64
	if bankAccountID != nil {
		if err := domain.ValidateBankAccountID(*bankAccountID); err != nil {
			return domain.BalanceAccountLink{}, err
		}
		account, err := s.repo.GetByID(ctx, *bankAccountID, false)
		if err != nil {
			return domain.BalanceAccountLink{}, err
		}
		_ = account
		value := *bankAccountID
		normalizedID = &value
	}

	if err := s.repo.UpsertBalanceLink(ctx, normalizedTarget, normalizedID); err != nil {
		return domain.BalanceAccountLink{}, err
	}

	links, err := s.repo.ListBalanceLinks(ctx)
	if err != nil {
		return domain.BalanceAccountLink{}, err
	}
	for _, link := range links {
		if link.Target == normalizedTarget {
			return link, nil
		}
	}

	return domain.BalanceAccountLink{Target: normalizedTarget}, nil
}

func (s *BankAccountService) ListBalanceLinks(ctx context.Context) ([]domain.BalanceAccountLink, error) {
	links, err := s.repo.ListBalanceLinks(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(links, func(i, j int) bool {
		return links[i].Target < links[j].Target
	})
	return links, nil
}

func matchesBankAccountLookup(account domain.BankAccount, lookup string) bool {
	alias := strings.ToLower(strings.TrimSpace(account.Alias))
	needle := strings.ToLower(strings.TrimSpace(lookup))
	if strings.Contains(alias, needle) {
		return true
	}
	return strings.Contains(account.Last4, lookup)
}

func sortBankAccountsDeterministic(accounts []domain.BankAccount) {
	sort.Slice(accounts, func(i, j int) bool {
		left := strings.ToLower(accounts[i].Alias)
		right := strings.ToLower(accounts[j].Alias)
		if left != right {
			return left < right
		}
		return accounts[i].ID < accounts[j].ID
	})
}
