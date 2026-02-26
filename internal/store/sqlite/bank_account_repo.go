package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"boring-budget/internal/domain"
	queries "boring-budget/internal/store/sqlite/sqlc"
)

type BankAccountRepo struct {
	db      *sql.DB
	queries *queries.Queries
}

func NewBankAccountRepo(db *sql.DB) *BankAccountRepo {
	return &BankAccountRepo{
		db:      db,
		queries: queries.New(db),
	}
}

func (r *BankAccountRepo) Add(ctx context.Context, input domain.BankAccountAddInput) (domain.BankAccount, error) {
	result, err := r.queries.CreateBankAccount(ctx, queries.CreateBankAccountParams{
		Alias:        strings.TrimSpace(input.Alias),
		Last4:        strings.TrimSpace(input.Last4),
		UpdatedAtUtc: nowRFC3339Nano(),
	})
	if err != nil {
		return domain.BankAccount{}, mapBankAccountWriteError("add bank account", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return domain.BankAccount{}, fmt.Errorf("add bank account read id: %w", err)
	}

	return r.GetByID(ctx, id, false)
}

func (r *BankAccountRepo) GetByID(ctx context.Context, id int64, includeDeleted bool) (domain.BankAccount, error) {
	if err := domain.ValidateBankAccountID(id); err != nil {
		return domain.BankAccount{}, err
	}

	var (
		row queries.BankAccount
		err error
	)
	if includeDeleted {
		row, err = r.queries.GetBankAccountByID(ctx, id)
	} else {
		row, err = r.queries.GetActiveBankAccountByID(ctx, id)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.BankAccount{}, domain.ErrBankAccountNotFound
		}
		return domain.BankAccount{}, fmt.Errorf("get bank account by id: %w", err)
	}

	return mapSQLCBankAccount(row), nil
}

func (r *BankAccountRepo) List(ctx context.Context, filter domain.BankAccountListFilter) ([]domain.BankAccount, error) {
	rows, err := r.queries.ListBankAccounts(ctx, boolAsInt64(filter.IncludeDeleted))
	if err != nil {
		return nil, fmt.Errorf("list bank accounts: %w", err)
	}

	accounts := make([]domain.BankAccount, 0, len(rows))
	for _, row := range rows {
		accounts = append(accounts, mapSQLCBankAccount(row))
	}

	return accounts, nil
}

func (r *BankAccountRepo) Search(ctx context.Context, lookup string, limit int32) ([]domain.BankAccount, error) {
	trimmed := strings.TrimSpace(lookup)
	if trimmed == "" {
		return nil, domain.ErrInvalidBankAccountLookup
	}
	if limit <= 0 {
		limit = 25
	}

	rows, err := r.queries.SearchActiveBankAccountsByLookup(ctx, queries.SearchActiveBankAccountsByLookupParams{
		LookupText: trimmed,
		LimitRows:  int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("search bank accounts: %w", err)
	}

	accounts := make([]domain.BankAccount, 0, len(rows))
	for _, row := range rows {
		accounts = append(accounts, mapSQLCBankAccount(row))
	}

	return accounts, nil
}

func (r *BankAccountRepo) Update(ctx context.Context, input domain.BankAccountUpdateInput) (domain.BankAccount, error) {
	if err := domain.ValidateBankAccountID(input.ID); err != nil {
		return domain.BankAccount{}, err
	}

	result, err := r.queries.UpdateBankAccountByID(ctx, queries.UpdateBankAccountByIDParams{
		SetAlias:     boolAsInt64(input.Alias != nil),
		Alias:        derefBankAccountString(input.Alias),
		SetLast4:     boolAsInt64(input.Last4 != nil),
		Last4:        derefBankAccountString(input.Last4),
		UpdatedAtUtc: nowRFC3339Nano(),
		ID:           input.ID,
	})
	if err != nil {
		return domain.BankAccount{}, mapBankAccountWriteError("update bank account", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return domain.BankAccount{}, fmt.Errorf("update bank account rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.BankAccount{}, domain.ErrBankAccountNotFound
	}

	return r.GetByID(ctx, input.ID, false)
}

func (r *BankAccountRepo) Delete(ctx context.Context, id int64) (domain.BankAccountDeleteResult, error) {
	if err := domain.ValidateBankAccountID(id); err != nil {
		return domain.BankAccountDeleteResult{}, err
	}

	deletedAtUTC := nowRFC3339Nano()
	result, err := r.queries.SoftDeleteBankAccount(ctx, queries.SoftDeleteBankAccountParams{
		DeletedAtUtc: sql.NullString{String: deletedAtUTC, Valid: true},
		UpdatedAtUtc: deletedAtUTC,
		ID:           id,
	})
	if err != nil {
		return domain.BankAccountDeleteResult{}, fmt.Errorf("delete bank account: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return domain.BankAccountDeleteResult{}, fmt.Errorf("delete bank account rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.BankAccountDeleteResult{}, domain.ErrBankAccountNotFound
	}

	return domain.BankAccountDeleteResult{
		BankAccountID: id,
		DeletedAtUTC:  deletedAtUTC,
	}, nil
}

func (r *BankAccountRepo) UpsertBalanceLink(ctx context.Context, target string, bankAccountID *int64) error {
	normalizedTarget, err := domain.NormalizeBalanceLinkTarget(target)
	if err != nil {
		return err
	}

	var nullableID sql.NullInt64
	if bankAccountID != nil {
		nullableID = sql.NullInt64{Int64: *bankAccountID, Valid: true}
	}

	_, err = r.queries.UpsertBalanceAccountLink(ctx, queries.UpsertBalanceAccountLinkParams{
		Target:        normalizedTarget,
		BankAccountID: nullableID,
		UpdatedAtUtc:  nowRFC3339Nano(),
	})
	if err != nil {
		return fmt.Errorf("upsert balance account link: %w", err)
	}

	return nil
}

func (r *BankAccountRepo) ListBalanceLinks(ctx context.Context) ([]domain.BalanceAccountLink, error) {
	rows, err := r.queries.ListBalanceAccountLinks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list balance account links: %w", err)
	}

	out := make([]domain.BalanceAccountLink, 0, len(rows))
	for _, row := range rows {
		link := domain.BalanceAccountLink{Target: row.Target}
		if row.BankAccountID.Valid && row.AccountAlias.Valid && row.AccountLast4.Valid {
			account := domain.BankAccount{
				ID:    row.BankAccountID.Int64,
				Alias: row.AccountAlias.String,
				Last4: row.AccountLast4.String,
			}
			link.BankAccount = &account
		}
		out = append(out, link)
	}
	return out, nil
}

func mapSQLCBankAccount(row queries.BankAccount) domain.BankAccount {
	return domain.BankAccount{
		ID:           row.ID,
		Alias:        row.Alias,
		Last4:        row.Last4,
		CreatedAtUTC: row.CreatedAtUtc,
		UpdatedAtUTC: row.UpdatedAtUtc,
		DeletedAtUTC: ptrStringFromNull(row.DeletedAtUtc),
	}
}

func mapBankAccountWriteError(operation string, err error) error {
	if isUniqueConstraintErr(err) {
		return domain.ErrBankAccountAliasConflict
	}
	if isBankAccountLast4ConstraintErr(err) {
		return domain.ErrBankAccountLast4Invalid
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func isBankAccountLast4ConstraintErr(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "constraint") {
		return false
	}
	return strings.Contains(msg, "last4") || strings.Contains(msg, "[0-9][0-9][0-9][0-9]")
}

func derefBankAccountString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
