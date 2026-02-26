package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"boring-budget/internal/cli/output"
	"boring-budget/internal/domain"
	"boring-budget/internal/service"
	sqlitestore "boring-budget/internal/store/sqlite"
	"github.com/spf13/cobra"
)

type bankAccountAddFlags struct {
	alias string
	last4 string
}

type bankAccountListFlags struct {
	lookup         string
	includeDeleted bool
}

type bankAccountUpdateFlags struct {
	alias string
	last4 string
}

type bankAccountLinkSetFlags struct {
	target       string
	accountIDRaw string
}

type bankAccountLinkClearFlags struct {
	target string
}

type bankAccountCLIError struct {
	Code    string
	Message string
	Details any
}

func (e *bankAccountCLIError) Error() string {
	if e == nil {
		return "bank-account command error"
	}
	return e.Message
}

func NewBankAccountCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bank-account",
		Short: "Manage bank accounts",
	}

	cmd.AddCommand(
		newBankAccountAddCmd(opts),
		newBankAccountListCmd(opts),
		newBankAccountUpdateCmd(opts),
		newBankAccountDeleteCmd(opts),
		newBankAccountLinkCmd(opts),
	)

	return cmd
}

func newBankAccountLinkCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Associate bank accounts to balances",
	}

	cmd.AddCommand(
		newBankAccountLinkSetCmd(opts),
		newBankAccountLinkClearCmd(opts),
		newBankAccountLinkListCmd(opts),
	)

	return cmd
}

func newBankAccountLinkSetCmd(opts *RootOptions) *cobra.Command {
	flags := &bankAccountLinkSetFlags{}

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set bank account association for general_balance or savings",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printBankAccountError(cmd, opts.Output, &bankAccountCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "bank-account link set does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			target, err := domain.NormalizeBalanceLinkTarget(flags.target)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}
			accountID, err := parsePositiveBankAccountID(flags.accountIDRaw, "account-id")
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			svc, err := newBankAccountService(opts)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			link, err := svc.SetBalanceLink(cmd.Context(), target, &accountID)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{
				"link": link,
			}, nil))
		},
	}

	cmd.Flags().StringVar(&flags.target, "target", "", "Balance target: general_balance|savings")
	cmd.Flags().StringVar(&flags.accountIDRaw, "account-id", "", "Bank account id")
	_ = cmd.MarkFlagRequired("target")
	_ = cmd.MarkFlagRequired("account-id")

	return cmd
}

func newBankAccountLinkClearCmd(opts *RootOptions) *cobra.Command {
	flags := &bankAccountLinkClearFlags{}

	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear bank account association for target",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printBankAccountError(cmd, opts.Output, &bankAccountCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "bank-account link clear does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			target, err := domain.NormalizeBalanceLinkTarget(flags.target)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			svc, err := newBankAccountService(opts)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			link, err := svc.SetBalanceLink(cmd.Context(), target, nil)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{
				"link": link,
			}, nil))
		},
	}

	cmd.Flags().StringVar(&flags.target, "target", "", "Balance target: general_balance|savings")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

func newBankAccountLinkListCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List current balance-to-bank-account links",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printBankAccountError(cmd, opts.Output, &bankAccountCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "bank-account link list does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newBankAccountService(opts)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			links, err := svc.ListBalanceLinks(cmd.Context())
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{
				"links": links,
				"count": len(links),
			}, nil))
		},
	}
}

func newBankAccountAddCmd(opts *RootOptions) *cobra.Command {
	flags := &bankAccountAddFlags{}

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a bank account",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printBankAccountError(cmd, opts.Output, &bankAccountCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "bank-account add does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			if strings.TrimSpace(flags.alias) == "" {
				return printBankAccountError(cmd, opts.Output, &bankAccountCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "alias is required",
					Details: map[string]any{"field": "alias"},
				})
			}
			if strings.TrimSpace(flags.last4) == "" {
				return printBankAccountError(cmd, opts.Output, &bankAccountCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "last4 is required",
					Details: map[string]any{"field": "last4"},
				})
			}

			svc, err := newBankAccountService(opts)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			account, err := svc.Add(cmd.Context(), domain.BankAccountAddInput{
				Alias: flags.alias,
				Last4: flags.last4,
			})
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{
				"bank_account": account,
			}, nil))
		},
	}

	cmd.Flags().StringVar(&flags.alias, "alias", "", "Unique account alias")
	cmd.Flags().StringVar(&flags.last4, "last4", "", "Account identifier last 4 digits")

	return cmd
}

func newBankAccountListCmd(opts *RootOptions) *cobra.Command {
	flags := &bankAccountListFlags{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List bank accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printBankAccountError(cmd, opts.Output, &bankAccountCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "bank-account list does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newBankAccountService(opts)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			accounts, err := svc.List(cmd.Context(), domain.BankAccountListFilter{
				Lookup:         flags.lookup,
				IncludeDeleted: flags.includeDeleted,
			})
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{
				"bank_accounts": accounts,
				"count":         len(accounts),
			}, nil))
		},
	}

	cmd.Flags().StringVar(&flags.lookup, "lookup", "", "Optional lookup text over alias/last4")
	cmd.Flags().BoolVar(&flags.includeDeleted, "include-deleted", false, "Include soft-deleted bank accounts")

	return cmd
}

func newBankAccountUpdateCmd(opts *RootOptions) *cobra.Command {
	flags := &bankAccountUpdateFlags{}

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update bank account metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return printBankAccountError(cmd, opts.Output, &bankAccountCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "bank-account update requires exactly one argument: <id>",
					Details: map[string]any{"required_args": []string{"id"}},
				})
			}

			id, err := parsePositiveBankAccountID(args[0], "id")
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			svc, err := newBankAccountService(opts)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			input := domain.BankAccountUpdateInput{ID: id}
			if cmd.Flags().Changed("alias") {
				value := flags.alias
				input.Alias = &value
			}
			if cmd.Flags().Changed("last4") {
				value := flags.last4
				input.Last4 = &value
			}

			account, err := svc.Update(cmd.Context(), input)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{
				"bank_account": account,
			}, nil))
		},
	}

	cmd.Flags().StringVar(&flags.alias, "alias", "", "New account alias")
	cmd.Flags().StringVar(&flags.last4, "last4", "", "New account identifier last 4 digits")

	return cmd
}

func newBankAccountDeleteCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft-delete a bank account",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return printBankAccountError(cmd, opts.Output, &bankAccountCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "bank-account delete requires exactly one argument: <id>",
					Details: map[string]any{"required_args": []string{"id"}},
				})
			}

			id, err := parsePositiveBankAccountID(args[0], "id")
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			svc, err := newBankAccountService(opts)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			deleted, err := svc.Delete(cmd.Context(), id)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{
				"bank_account_delete": deleted,
			}, nil))
		},
	}
}

func newBankAccountService(opts *RootOptions) (*service.BankAccountService, error) {
	if opts == nil || opts.db == nil {
		return nil, &bankAccountCLIError{
			Code:    "DB_ERROR",
			Message: "database operation failed",
			Details: map[string]any{"reason": "database connection unavailable"},
		}
	}

	svc, err := service.NewBankAccountService(sqlitestore.NewBankAccountRepo(opts.db))
	if err != nil {
		return nil, err
	}
	return svc, nil
}

func parsePositiveBankAccountID(raw, field string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, &bankAccountCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: fmt.Sprintf("%s is required", field),
			Details: map[string]any{"field": field},
		}
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, &bankAccountCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: fmt.Sprintf("%s must be a positive integer", field),
			Details: map[string]any{"field": field, "value": raw},
		}
	}

	return parsed, nil
}

func printBankAccountError(cmd *cobra.Command, format string, err error) error {
	if cmd == nil {
		return fmt.Errorf("nil command")
	}

	if err == nil {
		env := output.NewErrorEnvelope("INTERNAL_ERROR", "unexpected internal failure", map[string]any{}, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	var cliErr *bankAccountCLIError
	if errors.As(err, &cliErr) {
		env := output.NewErrorEnvelope(cliErr.Code, cliErr.Message, cliErr.Details, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	env := output.NewErrorEnvelope(codeFromBankAccountError(err), messageFromBankAccountError(err), map[string]any{"reason": err.Error()}, nil)
	return output.Print(cmd.OutOrStdout(), format, env)
}

func codeFromBankAccountError(err error) string {
	switch {
	case errors.Is(err, domain.ErrBankAccountNotFound):
		return "NOT_FOUND"
	case errors.Is(err, domain.ErrBankAccountAliasConflict):
		return "CONFLICT"
	case errors.Is(err, domain.ErrInvalidBankAccountID),
		errors.Is(err, domain.ErrInvalidBalanceLinkTarget),
		errors.Is(err, domain.ErrBankAccountAliasRequired),
		errors.Is(err, domain.ErrBankAccountLast4Invalid),
		errors.Is(err, domain.ErrNoBankAccountUpdateFields),
		errors.Is(err, domain.ErrInvalidBankAccountLookup):
		return "INVALID_ARGUMENT"
	default:
		return "DB_ERROR"
	}
}

func messageFromBankAccountError(err error) string {
	switch {
	case errors.Is(err, domain.ErrBankAccountNotFound):
		return "bank account not found"
	case errors.Is(err, domain.ErrBankAccountAliasConflict):
		return "bank account alias already exists"
	case errors.Is(err, domain.ErrInvalidBankAccountID):
		return "bank account id must be a positive integer"
	case errors.Is(err, domain.ErrInvalidBalanceLinkTarget):
		return "target must be one of: general_balance|savings"
	case errors.Is(err, domain.ErrBankAccountAliasRequired):
		return "alias is required"
	case errors.Is(err, domain.ErrBankAccountLast4Invalid):
		return "last4 must be exactly 4 digits"
	case errors.Is(err, domain.ErrNoBankAccountUpdateFields):
		return "at least one update field is required"
	case errors.Is(err, domain.ErrInvalidBankAccountLookup):
		return "lookup cannot be empty"
	default:
		return "database operation failed"
	}
}
