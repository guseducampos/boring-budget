package cli

import (
	"errors"
	"fmt"
	"strings"

	"boring-budget/internal/cli/output"
	"boring-budget/internal/domain"
	"boring-budget/internal/service"
	sqlitestore "boring-budget/internal/store/sqlite"
	"github.com/spf13/cobra"
)

type savingsAddFlags struct {
	amount                  string
	currency                string
	dateRaw                 string
	sourceAccountIDRaw      string
	destinationAccountIDRaw string
	accountIDRaw            string
	note                    string
}

type savingsShowFlags struct {
	scope   string
	fromRaw string
	toRaw   string
}

type savingsShowRequest struct {
	Scope           string
	FromUTC         string
	ToUTC           string
	IncludeLifetime bool
	IncludeRange    bool
}

type savingsCLIError struct {
	Code    string
	Message string
	Details any
}

type savingsBalanceRow struct {
	CurrencyCode        string `json:"currency_code"`
	GeneralBalanceMinor int64  `json:"general_balance_minor"`
	SavingsBalanceMinor int64  `json:"savings_balance_minor"`
	TotalBalanceMinor   int64  `json:"total_balance_minor"`
}

type savingsViewPayload struct {
	ByCurrency []savingsBalanceRow `json:"by_currency"`
}

type savingsRangePayload struct {
	FromUTC    string              `json:"from_utc,omitempty"`
	ToUTC      string              `json:"to_utc,omitempty"`
	ByCurrency []savingsBalanceRow `json:"by_currency"`
}

type savingsShowPayload struct {
	Scope          string                      `json:"scope"`
	Lifetime       *savingsViewPayload         `json:"lifetime,omitempty"`
	Range          *savingsRangePayload        `json:"range,omitempty"`
	LinkedAccounts []domain.BalanceAccountLink `json:"linked_accounts"`
}

func (e *savingsCLIError) Error() string {
	if e == nil {
		return "savings command error"
	}
	return e.Message
}

func NewSavingsCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "savings",
		Short: "Manage savings flows",
	}

	transferCmd := &cobra.Command{
		Use:   "transfer",
		Short: "Savings transfer operations",
	}
	transferCmd.AddCommand(newSavingsTransferAddCmd(opts))

	entryCmd := &cobra.Command{
		Use:   "entry",
		Short: "Direct savings entry operations",
	}
	entryCmd.AddCommand(newSavingsEntryAddCmd(opts))

	cmd.AddCommand(
		transferCmd,
		entryCmd,
		newSavingsShowCmd(opts),
	)

	return cmd
}

func newSavingsTransferAddCmd(opts *RootOptions) *cobra.Command {
	flags := &savingsAddFlags{}

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Move money from general into savings",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printSavingsError(cmd, savingsOutputFormat(opts), &savingsCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "savings transfer add does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newSavingsService(opts)
			if err != nil {
				return printSavingsError(cmd, savingsOutputFormat(opts), err)
			}

			input, err := buildSavingsAddInput(cmd, flags, true)
			if err != nil {
				return printSavingsError(cmd, savingsOutputFormat(opts), err)
			}

			event, err := svc.AddTransfer(cmd.Context(), input)
			if err != nil {
				return printSavingsError(cmd, savingsOutputFormat(opts), err)
			}

			return output.Print(cmd.OutOrStdout(), savingsOutputFormat(opts), output.NewSuccessEnvelope(map[string]any{
				"event": event,
			}, nil))
		},
	}

	cmd.Flags().StringVar(&flags.amount, "amount", "", "Amount in major units (e.g. 74.25)")
	cmd.Flags().StringVar(&flags.currency, "currency", defaultEntryCurrency, "ISO currency code (e.g. USD)")
	cmd.Flags().StringVar(&flags.dateRaw, "date", "", "Savings event date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.sourceAccountIDRaw, "source-account-id", "", "Optional source bank account ID")
	cmd.Flags().StringVar(&flags.destinationAccountIDRaw, "destination-account-id", "", "Optional destination bank account ID")
	cmd.Flags().StringVar(&flags.note, "note", "", "Optional note")

	return cmd
}

func newSavingsEntryAddCmd(opts *RootOptions) *cobra.Command {
	flags := &savingsAddFlags{}

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add money directly into savings",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printSavingsError(cmd, savingsOutputFormat(opts), &savingsCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "savings entry add does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newSavingsService(opts)
			if err != nil {
				return printSavingsError(cmd, savingsOutputFormat(opts), err)
			}

			input, err := buildSavingsAddInput(cmd, flags, false)
			if err != nil {
				return printSavingsError(cmd, savingsOutputFormat(opts), err)
			}

			event, err := svc.AddEntry(cmd.Context(), input)
			if err != nil {
				return printSavingsError(cmd, savingsOutputFormat(opts), err)
			}

			return output.Print(cmd.OutOrStdout(), savingsOutputFormat(opts), output.NewSuccessEnvelope(map[string]any{
				"event": event,
			}, nil))
		},
	}

	cmd.Flags().StringVar(&flags.amount, "amount", "", "Amount in major units (e.g. 74.25)")
	cmd.Flags().StringVar(&flags.currency, "currency", defaultEntryCurrency, "ISO currency code (e.g. USD)")
	cmd.Flags().StringVar(&flags.dateRaw, "date", "", "Savings event date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.accountIDRaw, "account-id", "", "Optional savings bank account ID")
	cmd.Flags().StringVar(&flags.note, "note", "", "Optional note")

	return cmd
}

func newSavingsShowCmd(opts *RootOptions) *cobra.Command {
	flags := &savingsShowFlags{}

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show savings and general balances",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printSavingsError(cmd, savingsOutputFormat(opts), &savingsCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "savings show does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			req, err := buildSavingsShowRequest(flags)
			if err != nil {
				return printSavingsError(cmd, savingsOutputFormat(opts), err)
			}

			svc, err := newSavingsService(opts)
			if err != nil {
				return printSavingsError(cmd, savingsOutputFormat(opts), err)
			}

			views, err := svc.Show(cmd.Context(), service.SavingsShowRequest{
				IncludeLifetime: req.IncludeLifetime,
				IncludeRange:    req.IncludeRange,
				RangeFromUTC:    req.FromUTC,
				RangeToUTC:      req.ToUTC,
			})
			if err != nil {
				return printSavingsError(cmd, savingsOutputFormat(opts), err)
			}

			payload := savingsShowPayload{
				Scope:          req.Scope,
				LinkedAccounts: []domain.BalanceAccountLink{},
			}
			if views.Lifetime != nil {
				payload.Lifetime = &savingsViewPayload{ByCurrency: toSavingsBalanceRows(views.Lifetime.ByCurrency)}
			}
			if views.Range != nil {
				payload.Range = &savingsRangePayload{
					FromUTC:    req.FromUTC,
					ToUTC:      req.ToUTC,
					ByCurrency: toSavingsBalanceRows(views.Range.ByCurrency),
				}
			}

			links, err := loadBalanceLinks(cmd.Context(), opts)
			if err == nil {
				payload.LinkedAccounts = links
			}

			return output.Print(cmd.OutOrStdout(), savingsOutputFormat(opts), output.NewSuccessEnvelope(payload, nil))
		},
	}

	cmd.Flags().StringVar(&flags.scope, "scope", balanceScopeBoth, "Savings scope: lifetime|range|both")
	cmd.Flags().StringVar(&flags.fromRaw, "from", "", "Filter start date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.toRaw, "to", "", "Filter end date (RFC3339 or YYYY-MM-DD)")

	return cmd
}

func newSavingsService(opts *RootOptions) (*service.SavingsService, error) {
	if opts == nil || opts.db == nil {
		return nil, &savingsCLIError{
			Code:    "DB_ERROR",
			Message: "database operation failed",
			Details: map[string]any{"reason": "database connection unavailable"},
		}
	}

	entryRepo := sqlitestore.NewEntryRepo(opts.db)
	savingsRepo := sqlitestore.NewSavingsRepo(opts.db)
	bankAccountRepo := sqlitestore.NewBankAccountRepo(opts.db)

	svc, err := service.NewSavingsService(
		entryRepo,
		savingsRepo,
		service.WithSavingsBalanceLinkReader(bankAccountRepo),
	)
	if err != nil {
		return nil, fmt.Errorf("savings service init: %w", err)
	}
	return svc, nil
}

func buildSavingsAddInput(cmd *cobra.Command, flags *savingsAddFlags, isTransfer bool) (service.SavingsAddInput, error) {
	if flags == nil {
		return service.SavingsAddInput{}, &savingsCLIError{
			Code:    "INTERNAL_ERROR",
			Message: "savings add flags unavailable",
			Details: map[string]any{},
		}
	}
	if cmd != nil && !cmd.Flags().Changed("amount") {
		return service.SavingsAddInput{}, &savingsCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "amount is required",
			Details: map[string]any{"field": "amount"},
		}
	}
	if strings.TrimSpace(flags.dateRaw) == "" {
		return service.SavingsAddInput{}, &savingsCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "date is required",
			Details: map[string]any{"field": "date"},
		}
	}

	amountMinor, err := domain.ParseMajorAmountToMinor(flags.amount, flags.currency)
	if err != nil {
		return service.SavingsAddInput{}, err
	}

	var sourceBankAccountID *int64
	var destinationBankAccountID *int64
	if isTransfer {
		if cmd != nil && cmd.Flags().Changed("source-account-id") {
			id, err := parsePositiveInt64(flags.sourceAccountIDRaw, "source-account-id")
			if err != nil {
				return service.SavingsAddInput{}, err
			}
			sourceBankAccountID = &id
		}
		if cmd != nil && cmd.Flags().Changed("destination-account-id") {
			id, err := parsePositiveInt64(flags.destinationAccountIDRaw, "destination-account-id")
			if err != nil {
				return service.SavingsAddInput{}, err
			}
			destinationBankAccountID = &id
		}
	} else if cmd != nil && cmd.Flags().Changed("account-id") {
		id, err := parsePositiveInt64(flags.accountIDRaw, "account-id")
		if err != nil {
			return service.SavingsAddInput{}, err
		}
		destinationBankAccountID = &id
	}

	return service.SavingsAddInput{
		AmountMinor:              amountMinor,
		CurrencyCode:             flags.currency,
		EventDateUTC:             flags.dateRaw,
		SourceBankAccountID:      sourceBankAccountID,
		DestinationBankAccountID: destinationBankAccountID,
		Note:                     flags.note,
	}, nil
}

func buildSavingsShowRequest(flags *savingsShowFlags) (savingsShowRequest, error) {
	if flags == nil {
		return savingsShowRequest{}, &savingsCLIError{
			Code:    "INTERNAL_ERROR",
			Message: "savings show flags unavailable",
			Details: map[string]any{},
		}
	}

	scope, err := normalizeSavingsScope(flags.scope)
	if err != nil {
		return savingsShowRequest{}, err
	}

	fromUTC, err := normalizeListDateBound(flags.fromRaw, false)
	if err != nil {
		return savingsShowRequest{}, &savingsCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "from must be RFC3339 or YYYY-MM-DD",
			Details: map[string]any{"field": "from", "value": flags.fromRaw},
		}
	}
	toUTC, err := normalizeListDateBound(flags.toRaw, true)
	if err != nil {
		return savingsShowRequest{}, &savingsCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "to must be RFC3339 or YYYY-MM-DD",
			Details: map[string]any{"field": "to", "value": flags.toRaw},
		}
	}
	if err := domain.ValidateDateRange(fromUTC, toUTC); err != nil {
		return savingsShowRequest{}, err
	}

	req := savingsShowRequest{
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

func normalizeSavingsScope(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return balanceScopeBoth, nil
	}

	switch normalized {
	case balanceScopeLifetime, balanceScopeRange, balanceScopeBoth:
		return normalized, nil
	default:
		return "", &savingsCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "scope must be one of: lifetime|range|both",
			Details: map[string]any{"field": "scope", "value": raw},
		}
	}
}

func savingsOutputFormat(opts *RootOptions) string {
	if opts == nil {
		return output.FormatHuman
	}
	return opts.Output
}

func toSavingsBalanceRows(rows []domain.SavingsCurrencyBalance) []savingsBalanceRow {
	if len(rows) == 0 {
		return []savingsBalanceRow{}
	}

	out := make([]savingsBalanceRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, savingsBalanceRow{
			CurrencyCode:        row.CurrencyCode,
			GeneralBalanceMinor: row.GeneralBalanceMinor,
			SavingsBalanceMinor: row.SavingsBalanceMinor,
			TotalBalanceMinor:   row.TotalBalanceMinor,
		})
	}
	return out
}

func printSavingsError(cmd *cobra.Command, format string, err error) error {
	if cmd == nil {
		return fmt.Errorf("nil command")
	}

	if err == nil {
		env := output.NewErrorEnvelope("INTERNAL_ERROR", "unexpected internal failure", map[string]any{}, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	var cliErr *savingsCLIError
	if errors.As(err, &cliErr) {
		env := output.NewErrorEnvelope(cliErr.Code, cliErr.Message, cliErr.Details, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	env := output.NewErrorEnvelope(codeFromSavingsError(err), messageFromSavingsError(err), map[string]any{"reason": err.Error()}, nil)
	return output.Print(cmd.OutOrStdout(), format, env)
}

func codeFromSavingsError(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidCurrencyCode):
		return "INVALID_CURRENCY_CODE"
	case errors.Is(err, domain.ErrInvalidDateRange):
		return "INVALID_DATE_RANGE"
	case errors.Is(err, domain.ErrInvalidAmount),
		errors.Is(err, domain.ErrInvalidAmountPrecision),
		errors.Is(err, domain.ErrAmountOverflow),
		errors.Is(err, domain.ErrInvalidAmountMinor),
		errors.Is(err, domain.ErrInvalidTransactionDate),
		errors.Is(err, domain.ErrInvalidBankAccountID),
		errors.Is(err, domain.ErrInvalidSavingsEventType):
		return "INVALID_ARGUMENT"
	case errors.Is(err, domain.ErrBankAccountNotFound):
		return "NOT_FOUND"
	default:
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed") {
			return "CONFLICT"
		}
		return "DB_ERROR"
	}
}

func messageFromSavingsError(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidCurrencyCode):
		return "currency must be a 3-letter ISO code"
	case errors.Is(err, domain.ErrInvalidDateRange):
		return "from must be less than or equal to to"
	case errors.Is(err, domain.ErrInvalidAmount):
		return "amount must be a valid decimal number"
	case errors.Is(err, domain.ErrInvalidAmountPrecision):
		return "amount has too many decimal places for currency"
	case errors.Is(err, domain.ErrAmountOverflow):
		return "amount is too large"
	case errors.Is(err, domain.ErrInvalidAmountMinor):
		return "amount must be greater than zero"
	case errors.Is(err, domain.ErrInvalidTransactionDate):
		return "date/from/to must be RFC3339 or YYYY-MM-DD"
	case errors.Is(err, domain.ErrInvalidBankAccountID):
		return "account id must be a positive integer"
	case errors.Is(err, domain.ErrInvalidSavingsEventType):
		return "event_type is invalid"
	case errors.Is(err, domain.ErrBankAccountNotFound):
		return "bank account not found"
	default:
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed") {
			return "conflict while processing savings command"
		}
		return "database operation failed"
	}
}
