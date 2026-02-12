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

type capSetFlags struct {
	monthRaw    string
	amount      string
	currencyRaw string
}

type capMonthFlags struct {
	monthRaw string
}

type capCLIError struct {
	Code    string
	Message string
	Details any
}

func (e *capCLIError) Error() string {
	if e == nil {
		return "cap command error"
	}
	return e.Message
}

func NewCapCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cap",
		Short: "Manage monthly expense caps",
	}

	cmd.AddCommand(
		newCapSetCmd(opts),
		newCapShowCmd(opts),
		newCapHistoryCmd(opts),
	)

	return cmd
}

func newCapSetCmd(opts *RootOptions) *cobra.Command {
	flags := &capSetFlags{}

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Create or update a monthly cap",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printCapError(cmd, capOutputFormat(opts), &capCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "cap set does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newCapService(opts)
			if err != nil {
				return printCapError(cmd, capOutputFormat(opts), err)
			}

			input, err := buildCapSetInput(cmd, flags)
			if err != nil {
				return printCapError(cmd, capOutputFormat(opts), err)
			}

			capValue, change, err := svc.Set(cmd.Context(), input)
			if err != nil {
				return printCapError(cmd, capOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(map[string]any{
				"cap":        capValue,
				"cap_change": change,
			}, nil)
			return output.Print(cmd.OutOrStdout(), capOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.monthRaw, "month", "", "Target month in YYYY-MM")
	cmd.Flags().StringVar(&flags.amount, "amount", "", "Cap amount in major units (e.g. 500.00)")
	cmd.Flags().StringVar(&flags.currencyRaw, "currency", defaultEntryCurrency, "ISO currency code (e.g. USD)")

	return cmd
}

func newCapShowCmd(opts *RootOptions) *cobra.Command {
	flags := &capMonthFlags{}

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show the current cap for a month",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printCapError(cmd, capOutputFormat(opts), &capCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "cap show does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newCapService(opts)
			if err != nil {
				return printCapError(cmd, capOutputFormat(opts), err)
			}

			monthKey, err := normalizeMonthKey(flags.monthRaw)
			if err != nil {
				return printCapError(cmd, capOutputFormat(opts), err)
			}

			capValue, err := svc.Show(cmd.Context(), monthKey)
			if err != nil {
				return printCapError(cmd, capOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(map[string]any{"cap": capValue}, nil)
			return output.Print(cmd.OutOrStdout(), capOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.monthRaw, "month", "", "Target month in YYYY-MM")

	return cmd
}

func newCapHistoryCmd(opts *RootOptions) *cobra.Command {
	flags := &capMonthFlags{}

	cmd := &cobra.Command{
		Use:   "history",
		Short: "List cap changes for a month",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printCapError(cmd, capOutputFormat(opts), &capCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "cap history does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newCapService(opts)
			if err != nil {
				return printCapError(cmd, capOutputFormat(opts), err)
			}

			monthKey, err := normalizeMonthKey(flags.monthRaw)
			if err != nil {
				return printCapError(cmd, capOutputFormat(opts), err)
			}

			changes, err := svc.History(cmd.Context(), monthKey)
			if err != nil {
				return printCapError(cmd, capOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(map[string]any{
				"month_key": monthKey,
				"changes":   changes,
				"count":     len(changes),
			}, nil)
			return output.Print(cmd.OutOrStdout(), capOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.monthRaw, "month", "", "Target month in YYYY-MM")

	return cmd
}

func newCapService(opts *RootOptions) (*service.CapService, error) {
	if opts == nil || opts.db == nil {
		return nil, &capCLIError{
			Code:    "DB_ERROR",
			Message: "database operation failed",
			Details: map[string]any{"reason": "database connection unavailable"},
		}
	}

	repo := sqlitestore.NewCapRepo(opts.db)
	svc, err := service.NewCapService(repo)
	if err != nil {
		return nil, fmt.Errorf("cap service init: %w", err)
	}
	return svc, nil
}

func buildCapSetInput(cmd *cobra.Command, flags *capSetFlags) (domain.CapSetInput, error) {
	if flags == nil {
		return domain.CapSetInput{}, &capCLIError{
			Code:    "INTERNAL_ERROR",
			Message: "cap set flags unavailable",
			Details: map[string]any{},
		}
	}
	if cmd != nil && !cmd.Flags().Changed("amount") {
		return domain.CapSetInput{}, &capCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "amount is required",
			Details: map[string]any{"field": "amount"},
		}
	}

	monthKey, err := normalizeMonthKey(flags.monthRaw)
	if err != nil {
		return domain.CapSetInput{}, err
	}

	amountMinor, err := domain.ParseMajorAmountToMinor(flags.amount, flags.currencyRaw)
	if err != nil {
		return domain.CapSetInput{}, err
	}

	return domain.CapSetInput{
		MonthKey:     monthKey,
		AmountMinor:  amountMinor,
		CurrencyCode: flags.currencyRaw,
	}, nil
}

func normalizeMonthKey(raw string) (string, error) {
	month := strings.TrimSpace(raw)
	if month == "" {
		return "", &capCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "month is required",
			Details: map[string]any{"field": "month"},
		}
	}

	normalized, err := domain.NormalizeMonthKey(month)
	if err != nil {
		return "", &capCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "month must use YYYY-MM",
			Details: map[string]any{"field": "month", "value": raw},
		}
	}
	return normalized, nil
}

func capOutputFormat(opts *RootOptions) string {
	if opts == nil {
		return output.FormatHuman
	}
	return opts.Output
}

func printCapError(cmd *cobra.Command, format string, err error) error {
	if cmd == nil {
		return fmt.Errorf("nil command")
	}

	if err == nil {
		env := output.NewErrorEnvelope("INTERNAL_ERROR", "unexpected internal failure", map[string]any{}, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	var cliErr *capCLIError
	if errors.As(err, &cliErr) {
		env := output.NewErrorEnvelope(cliErr.Code, cliErr.Message, cliErr.Details, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	env := output.NewErrorEnvelope(codeFromCapError(err), messageFromCapError(err), map[string]any{"reason": err.Error()}, nil)
	return output.Print(cmd.OutOrStdout(), format, env)
}

func codeFromCapError(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidMonthKey),
		errors.Is(err, domain.ErrInvalidCapAmount),
		errors.Is(err, domain.ErrInvalidAmount),
		errors.Is(err, domain.ErrInvalidAmountPrecision),
		errors.Is(err, domain.ErrAmountOverflow):
		return "INVALID_ARGUMENT"
	case errors.Is(err, domain.ErrInvalidCurrencyCode):
		return "INVALID_CURRENCY_CODE"
	case errors.Is(err, domain.ErrCapNotFound):
		return "NOT_FOUND"
	default:
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "unique constraint") || strings.Contains(message, "constraint failed") {
			return "CONFLICT"
		}
		return "DB_ERROR"
	}
}

func messageFromCapError(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidMonthKey):
		return "month must use YYYY-MM"
	case errors.Is(err, domain.ErrInvalidAmount):
		return "amount must be a valid decimal number"
	case errors.Is(err, domain.ErrInvalidAmountPrecision):
		return "amount has too many decimal places for currency"
	case errors.Is(err, domain.ErrAmountOverflow):
		return "amount is too large"
	case errors.Is(err, domain.ErrInvalidCapAmount):
		return "amount must be greater than zero"
	case errors.Is(err, domain.ErrInvalidCurrencyCode):
		return "currency must be a 3-letter ISO code"
	case errors.Is(err, domain.ErrCapNotFound):
		return "cap not found"
	default:
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "unique constraint") || strings.Contains(message, "constraint failed") {
			return "conflict while processing cap"
		}
		return "database operation failed"
	}
}
