package cli

import (
	"fmt"
	"strings"

	"boring-budget/internal/cli/output"
	"boring-budget/internal/domain"
	"boring-budget/internal/service"
	sqlitestore "boring-budget/internal/store/sqlite"
	"github.com/spf13/cobra"
)

type setupInitFlags struct {
	defaultCurrency      string
	timezone             string
	openingBalance       string
	openingBalanceCode   string
	openingBalanceDate   string
	currentMonthCap      string
	currentMonthCapCode  string
	currentMonthCapMonth string
}

func NewSetupCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Initialize and inspect onboarding settings",
	}

	cmd.AddCommand(
		newSetupInitCmd(opts),
		newSetupShowCmd(opts),
	)

	return cmd
}

func newSetupInitCmd(opts *RootOptions) *cobra.Command {
	flags := &setupInitFlags{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize user settings and optional opening data",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printReportError(cmd, reportOutputFormat(opts), &reportCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "setup init does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			if strings.TrimSpace(flags.defaultCurrency) == "" {
				return printReportError(cmd, reportOutputFormat(opts), &reportCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "default-currency is required",
					Details: map[string]any{"field": "default-currency"},
				})
			}
			if strings.TrimSpace(flags.timezone) == "" {
				return printReportError(cmd, reportOutputFormat(opts), &reportCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "timezone is required",
					Details: map[string]any{"field": "timezone"},
				})
			}
			var openingBalanceMinor int64
			if cmd.Flags().Changed("opening-balance") {
				openingBalanceCurrency := flags.openingBalanceCode
				if strings.TrimSpace(openingBalanceCurrency) == "" {
					openingBalanceCurrency = flags.defaultCurrency
				}

				parsedOpeningBalanceMinor, err := domain.ParseMajorAmountToMinor(flags.openingBalance, openingBalanceCurrency)
				if err != nil {
					return printReportError(cmd, reportOutputFormat(opts), err)
				}
				openingBalanceMinor = parsedOpeningBalanceMinor
			}

			var currentMonthCapMinor int64
			if cmd.Flags().Changed("month-cap") {
				monthCapCurrency := flags.currentMonthCapCode
				if strings.TrimSpace(monthCapCurrency) == "" {
					monthCapCurrency = flags.defaultCurrency
				}

				parsedCurrentMonthCapMinor, err := domain.ParseMajorAmountToMinor(flags.currentMonthCap, monthCapCurrency)
				if err != nil {
					return printReportError(cmd, reportOutputFormat(opts), err)
				}
				currentMonthCapMinor = parsedCurrentMonthCapMinor
			}

			setupSvc, err := newSetupService(opts)
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			result, err := setupSvc.Init(cmd.Context(), service.SetupInitInput{
				DefaultCurrencyCode:  flags.defaultCurrency,
				DisplayTimezone:      flags.timezone,
				OpeningBalanceMinor:  openingBalanceMinor,
				OpeningBalanceCode:   flags.openingBalanceCode,
				OpeningBalanceDate:   flags.openingBalanceDate,
				CurrentMonthCapMinor: currentMonthCapMinor,
				CurrentMonthCapCode:  flags.currentMonthCapCode,
				CurrentMonthKey:      flags.currentMonthCapMonth,
			})
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(result, toOutputWarnings(result.OpeningWarnings))
			return output.Print(cmd.OutOrStdout(), reportOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.defaultCurrency, "default-currency", "", "Default currency code (required, e.g. USD)")
	cmd.Flags().StringVar(&flags.timezone, "timezone", "", "Display timezone (required, e.g. America/New_York)")
	cmd.Flags().StringVar(&flags.openingBalance, "opening-balance", "", "Optional opening balance in major units (e.g. 1000.00)")
	cmd.Flags().StringVar(&flags.openingBalanceCode, "opening-balance-currency", "", "Optional opening balance currency (defaults to default-currency)")
	cmd.Flags().StringVar(&flags.openingBalanceDate, "opening-balance-date", "", "Optional opening balance date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.currentMonthCap, "month-cap", "", "Optional current month cap in major units (e.g. 500.00)")
	cmd.Flags().StringVar(&flags.currentMonthCapCode, "month-cap-currency", "", "Optional month cap currency (defaults to default-currency)")
	cmd.Flags().StringVar(&flags.currentMonthCapMonth, "month-cap-month", "", "Optional month cap target month (YYYY-MM, defaults to current month)")

	return cmd
}

func newSetupShowCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printReportError(cmd, reportOutputFormat(opts), &reportCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "setup show does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			setupSvc, err := newSetupService(opts)
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			settings, err := setupSvc.Show(cmd.Context())
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(map[string]any{"settings": settings}, nil)
			return output.Print(cmd.OutOrStdout(), reportOutputFormat(opts), env)
		},
	}
}

func newSetupService(opts *RootOptions) (*service.SetupService, error) {
	if opts == nil || opts.db == nil {
		return nil, &reportCLIError{
			Code:    "DB_ERROR",
			Message: "database operation failed",
			Details: map[string]any{"reason": "database connection unavailable"},
		}
	}

	settingsRepo := sqlitestore.NewSettingsRepo(opts.db)
	entryRepo := sqlitestore.NewEntryRepo(opts.db)
	capRepo := sqlitestore.NewCapRepo(opts.db)

	entrySvc, err := service.NewEntryService(entryRepo, service.WithEntryCapLookup(capRepo))
	if err != nil {
		return nil, fmt.Errorf("entry service init: %w", err)
	}
	capSvc, err := service.NewCapService(capRepo)
	if err != nil {
		return nil, fmt.Errorf("cap service init: %w", err)
	}

	setupSvc, err := service.NewSetupService(settingsRepo, entrySvc, capSvc)
	if err != nil {
		return nil, fmt.Errorf("setup service init: %w", err)
	}

	return setupSvc, nil
}
