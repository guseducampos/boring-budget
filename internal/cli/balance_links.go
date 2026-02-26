package cli

import (
	"context"

	"boring-budget/internal/domain"
	"boring-budget/internal/service"
	sqlitestore "boring-budget/internal/store/sqlite"
)

func loadBalanceLinks(ctx context.Context, opts *RootOptions) ([]domain.BalanceAccountLink, error) {
	if opts == nil || opts.db == nil {
		return []domain.BalanceAccountLink{}, nil
	}

	bankAccountSvc, err := service.NewBankAccountService(sqlitestore.NewBankAccountRepo(opts.db))
	if err != nil {
		return nil, err
	}

	links, err := bankAccountSvc.ListBalanceLinks(ctx)
	if err != nil {
		return nil, err
	}
	if links == nil {
		return []domain.BalanceAccountLink{}, nil
	}
	return links, nil
}
