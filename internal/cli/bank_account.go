package cli

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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

type bankAccountBalanceShowFlags struct {
	scope   string
	fromRaw string
	toRaw   string
}

type bankAccountBalanceShowRequest struct {
	Scope           string
	FromUTC         string
	ToUTC           string
	IncludeLifetime bool
	IncludeRange    bool
}

type bankAccountCurrencyBalance struct {
	CurrencyCode        string `json:"currency_code"`
	GeneralBalanceMinor int64  `json:"general_balance_minor"`
	SavingsBalanceMinor int64  `json:"savings_balance_minor"`
	TotalBalanceMinor   int64  `json:"total_balance_minor"`
}

type bankAccountBalanceAccount struct {
	AccountID  int64                        `json:"account_id"`
	Alias      string                       `json:"alias"`
	Last4      string                       `json:"last4"`
	ByCurrency []bankAccountCurrencyBalance `json:"by_currency"`
}

type bankAccountBalanceView struct {
	Accounts []bankAccountBalanceAccount `json:"accounts"`
}

type bankAccountBalanceRangeView struct {
	FromUTC  string                      `json:"from_utc,omitempty"`
	ToUTC    string                      `json:"to_utc,omitempty"`
	Accounts []bankAccountBalanceAccount `json:"accounts"`
}

type bankAccountBalancePayload struct {
	Scope    string                       `json:"scope"`
	Links    []domain.BalanceAccountLink  `json:"links"`
	Lifetime *bankAccountBalanceView      `json:"lifetime,omitempty"`
	Range    *bankAccountBalanceRangeView `json:"range,omitempty"`
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
		newBankAccountBalanceCmd(opts),
	)

	return cmd
}

func newBankAccountBalanceCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "balance",
		Short: "Show balance per bank account",
	}
	cmd.AddCommand(newBankAccountBalanceShowCmd(opts))
	return cmd
}

func newBankAccountBalanceShowCmd(opts *RootOptions) *cobra.Command {
	flags := &bankAccountBalanceShowFlags{}

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show per-account balances from attributed entries and savings events",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printBankAccountError(cmd, opts.Output, &bankAccountCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "bank-account balance show does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			req, err := buildBankAccountBalanceShowRequest(flags)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			bankSvc, err := newBankAccountService(opts)
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}
			entryRepo := sqlitestore.NewEntryRepo(opts.db)
			savingsRepo := sqlitestore.NewSavingsRepo(opts.db)

			accounts, err := bankSvc.List(cmd.Context(), domain.BankAccountListFilter{IncludeDeleted: false})
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}
			links, err := bankSvc.ListBalanceLinks(cmd.Context())
			if err != nil {
				return printBankAccountError(cmd, opts.Output, err)
			}

			payload := bankAccountBalancePayload{
				Scope: req.Scope,
				Links: links,
			}
			if req.IncludeLifetime {
				lifetimeEntries, err := entryRepo.List(cmd.Context(), domain.EntryListFilter{})
				if err != nil {
					return printBankAccountError(cmd, opts.Output, err)
				}
				lifetimeEvents, err := savingsRepo.ListEvents(cmd.Context(), domain.SavingsEventListFilter{})
				if err != nil {
					return printBankAccountError(cmd, opts.Output, err)
				}
				balances, err := buildAttributedAccountBalances(accounts, links, lifetimeEntries, lifetimeEvents)
				if err != nil {
					return printBankAccountError(cmd, opts.Output, err)
				}
				payload.Lifetime = &bankAccountBalanceView{
					Accounts: balances,
				}
			}
			if req.IncludeRange {
				rangeEntries, err := entryRepo.List(cmd.Context(), domain.EntryListFilter{
					DateFromUTC: req.FromUTC,
					DateToUTC:   req.ToUTC,
				})
				if err != nil {
					return printBankAccountError(cmd, opts.Output, err)
				}
				rangeEvents, err := savingsRepo.ListEvents(cmd.Context(), domain.SavingsEventListFilter{
					DateFromUTC: req.FromUTC,
					DateToUTC:   req.ToUTC,
				})
				if err != nil {
					return printBankAccountError(cmd, opts.Output, err)
				}
				balances, err := buildAttributedAccountBalances(accounts, links, rangeEntries, rangeEvents)
				if err != nil {
					return printBankAccountError(cmd, opts.Output, err)
				}
				payload.Range = &bankAccountBalanceRangeView{
					FromUTC:  req.FromUTC,
					ToUTC:    req.ToUTC,
					Accounts: balances,
				}
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(payload, nil))
		},
	}

	cmd.Flags().StringVar(&flags.scope, "scope", balanceScopeBoth, "Balance scope: lifetime|range|both")
	cmd.Flags().StringVar(&flags.fromRaw, "from", "", "Filter start date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.toRaw, "to", "", "Filter end date (RFC3339 or YYYY-MM-DD)")

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
		errors.Is(err, domain.ErrInvalidDateRange),
		errors.Is(err, domain.ErrInvalidTransactionDate),
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
	case errors.Is(err, domain.ErrInvalidDateRange):
		return "from must be less than or equal to to"
	case errors.Is(err, domain.ErrInvalidTransactionDate):
		return "date/from/to must be RFC3339 or YYYY-MM-DD"
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

func buildBankAccountBalanceShowRequest(flags *bankAccountBalanceShowFlags) (bankAccountBalanceShowRequest, error) {
	if flags == nil {
		return bankAccountBalanceShowRequest{}, &bankAccountCLIError{
			Code:    "INTERNAL_ERROR",
			Message: "bank-account balance flags unavailable",
			Details: map[string]any{},
		}
	}

	scope, err := normalizeSavingsScope(flags.scope)
	if err != nil {
		return bankAccountBalanceShowRequest{}, &bankAccountCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "scope must be one of: lifetime|range|both",
			Details: map[string]any{"field": "scope", "value": flags.scope},
		}
	}

	fromUTC, err := normalizeListDateBound(flags.fromRaw, false)
	if err != nil {
		return bankAccountBalanceShowRequest{}, &bankAccountCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "from must be RFC3339 or YYYY-MM-DD",
			Details: map[string]any{"field": "from", "value": flags.fromRaw},
		}
	}
	toUTC, err := normalizeListDateBound(flags.toRaw, true)
	if err != nil {
		return bankAccountBalanceShowRequest{}, &bankAccountCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "to must be RFC3339 or YYYY-MM-DD",
			Details: map[string]any{"field": "to", "value": flags.toRaw},
		}
	}
	if err := domain.ValidateDateRange(fromUTC, toUTC); err != nil {
		return bankAccountBalanceShowRequest{}, err
	}

	req := bankAccountBalanceShowRequest{
		Scope:   scope,
		FromUTC: fromUTC,
		ToUTC:   toUTC,
	}
	switch scope {
	case balanceScopeLifetime:
		req.IncludeLifetime = true
	case balanceScopeRange:
		req.IncludeRange = true
	case balanceScopeBoth:
		req.IncludeLifetime = true
		req.IncludeRange = true
	}

	return req, nil
}

type bankAccountReplayKind int

const (
	bankAccountReplayKindEntry bankAccountReplayKind = iota
	bankAccountReplayKindEvent
)

type bankAccountReplayItem struct {
	kind                     bankAccountReplayKind
	id                       int64
	date                     int64
	currencyCode             string
	amountMinor              int64
	entryType                string
	entryBankAccountID       *int64
	eventType                string
	eventSourceBankAccountID *int64
	eventTargetBankAccountID *int64
}

type bankAccountBalanceState struct {
	general int64
	savings int64
}

type bankAccountCurrencyBucket struct {
	general int64
	savings int64
}

func buildAttributedAccountBalances(accounts []domain.BankAccount, links []domain.BalanceAccountLink, entries []domain.Entry, events []domain.SavingsEvent) ([]bankAccountBalanceAccount, error) {
	replayItems, err := buildBankAccountReplay(entries, events)
	if err != nil {
		return nil, err
	}

	generalLinkedAccountID, savingsLinkedAccountID := linkedAccountIDs(links)
	accountBuckets := make(map[int64]map[string]bankAccountCurrencyBucket, len(accounts))
	for _, account := range accounts {
		accountBuckets[account.ID] = map[string]bankAccountCurrencyBucket{}
	}
	stateByCurrency := map[string]bankAccountBalanceState{}

	for _, item := range replayItems {
		state := stateByCurrency[item.currencyCode]

		switch item.kind {
		case bankAccountReplayKindEntry:
			switch item.entryType {
			case domain.EntryTypeIncome:
				state.general += item.amountMinor
				accountID := resolveAccountID(item.entryBankAccountID, generalLinkedAccountID)
				applyAccountBalanceDelta(accountBuckets, accountID, item.currencyCode, item.amountMinor, 0)
			case domain.EntryTypeExpense:
				generalUsed, savingsUsed, deficit := consumeExpenseFromBalances(&state, item.amountMinor)
				generalAccountID := resolveAccountID(item.entryBankAccountID, generalLinkedAccountID)
				savingsAccountID := savingsLinkedAccountID
				if savingsAccountID == nil {
					savingsAccountID = generalAccountID
				}
				applyAccountBalanceDelta(accountBuckets, generalAccountID, item.currencyCode, -generalUsed-deficit, 0)
				applyAccountBalanceDelta(accountBuckets, savingsAccountID, item.currencyCode, 0, -savingsUsed)
			}
		case bankAccountReplayKindEvent:
			switch item.eventType {
			case domain.SavingsEventTypeTransferToSavings:
				state.general -= item.amountMinor
				state.savings += item.amountMinor
				sourceID := resolveAccountID(item.eventSourceBankAccountID, generalLinkedAccountID)
				targetID := resolveAccountID(item.eventTargetBankAccountID, savingsLinkedAccountID)
				applyAccountBalanceDelta(accountBuckets, sourceID, item.currencyCode, -item.amountMinor, 0)
				applyAccountBalanceDelta(accountBuckets, targetID, item.currencyCode, 0, item.amountMinor)
			case domain.SavingsEventTypeIndependentAdd:
				state.savings += item.amountMinor
				targetID := resolveAccountID(item.eventTargetBankAccountID, savingsLinkedAccountID)
				applyAccountBalanceDelta(accountBuckets, targetID, item.currencyCode, 0, item.amountMinor)
			}
		}

		stateByCurrency[item.currencyCode] = state
	}

	out := make([]bankAccountBalanceAccount, 0, len(accounts))
	for _, account := range accounts {
		currencyMap := accountBuckets[account.ID]
		currencies := make([]string, 0, len(currencyMap))
		for code := range currencyMap {
			currencies = append(currencies, code)
		}
		sort.Strings(currencies)

		rows := make([]bankAccountCurrencyBalance, 0, len(currencies))
		for _, code := range currencies {
			bucket := currencyMap[code]
			rows = append(rows, bankAccountCurrencyBalance{
				CurrencyCode:        code,
				GeneralBalanceMinor: bucket.general,
				SavingsBalanceMinor: bucket.savings,
				TotalBalanceMinor:   bucket.general + bucket.savings,
			})
		}

		out = append(out, bankAccountBalanceAccount{
			AccountID:  account.ID,
			Alias:      account.Alias,
			Last4:      account.Last4,
			ByCurrency: rows,
		})
	}

	return out, nil
}

func buildBankAccountReplay(entries []domain.Entry, events []domain.SavingsEvent) ([]bankAccountReplayItem, error) {
	items := make([]bankAccountReplayItem, 0, len(entries)+len(events))
	for _, entry := range entries {
		dateValue, err := parseBankAccountBalanceDate(entry.TransactionDateUTC)
		if err != nil {
			return nil, err
		}
		items = append(items, bankAccountReplayItem{
			kind:               bankAccountReplayKindEntry,
			id:                 entry.ID,
			date:               dateValue,
			currencyCode:       entry.CurrencyCode,
			amountMinor:        entry.AmountMinor,
			entryType:          entry.Type,
			entryBankAccountID: entry.BankAccountID,
		})
	}
	for _, event := range events {
		dateValue, err := parseBankAccountBalanceDate(event.EventDateUTC)
		if err != nil {
			return nil, err
		}
		items = append(items, bankAccountReplayItem{
			kind:                     bankAccountReplayKindEvent,
			id:                       event.ID,
			date:                     dateValue,
			currencyCode:             event.CurrencyCode,
			amountMinor:              event.AmountMinor,
			eventType:                event.EventType,
			eventSourceBankAccountID: event.SourceBankAccountID,
			eventTargetBankAccountID: event.DestinationBankAccountID,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].date != items[j].date {
			return items[i].date < items[j].date
		}
		if items[i].id != items[j].id {
			return items[i].id < items[j].id
		}
		return items[i].kind < items[j].kind
	})

	return items, nil
}

func parseBankAccountBalanceDate(value string) (int64, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC().UnixNano(), nil
		}
	}
	return 0, domain.ErrInvalidTransactionDate
}

func consumeExpenseFromBalances(state *bankAccountBalanceState, amountMinor int64) (int64, int64, int64) {
	if state == nil || amountMinor <= 0 {
		return 0, 0, 0
	}

	generalAvailable := state.general
	if generalAvailable < 0 {
		generalAvailable = 0
	}
	generalUsed := minInt64(amountMinor, generalAvailable)

	remaining := amountMinor - generalUsed
	savingsAvailable := state.savings
	if savingsAvailable < 0 {
		savingsAvailable = 0
	}
	savingsUsed := minInt64(remaining, savingsAvailable)

	deficit := remaining - savingsUsed
	state.general = state.general - generalUsed - deficit
	state.savings = state.savings - savingsUsed

	return generalUsed, savingsUsed, deficit
}

func applyAccountBalanceDelta(accountBuckets map[int64]map[string]bankAccountCurrencyBucket, accountID *int64, currencyCode string, generalDelta, savingsDelta int64) {
	if accountID == nil {
		return
	}
	bucketsByCurrency, ok := accountBuckets[*accountID]
	if !ok {
		return
	}
	bucket := bucketsByCurrency[currencyCode]
	bucket.general += generalDelta
	bucket.savings += savingsDelta
	bucketsByCurrency[currencyCode] = bucket
}

func resolveAccountID(primary, fallback *int64) *int64 {
	if primary != nil {
		value := *primary
		return &value
	}
	if fallback != nil {
		value := *fallback
		return &value
	}
	return nil
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func linkedAccountIDs(links []domain.BalanceAccountLink) (*int64, *int64) {
	var generalAccountID *int64
	var savingsAccountID *int64

	for _, link := range links {
		if link.BankAccount == nil {
			continue
		}
		switch link.Target {
		case domain.BalanceLinkTargetGeneral:
			value := link.BankAccount.ID
			generalAccountID = &value
		case domain.BalanceLinkTargetSavings:
			value := link.BankAccount.ID
			savingsAccountID = &value
		}
	}

	return generalAccountID, savingsAccountID
}
