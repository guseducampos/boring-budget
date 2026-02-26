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

type scheduleAddFlags struct {
	name          string
	amount        string
	currency      string
	day           int
	startMonthRaw string
	endMonthRaw   string
	categoryIDRaw string
	note          string
}

type scheduleListFlags struct {
	includeDeleted bool
}

type scheduleRunFlags struct {
	throughDateRaw string
	scheduleIDRaw  string
	dryRun         bool
}

type scheduleCLIError struct {
	Code    string
	Message string
	Details any
}

func (e *scheduleCLIError) Error() string {
	if e == nil {
		return "schedule command error"
	}
	return e.Message
}

func NewScheduleCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage scheduled monthly payments",
	}

	cmd.AddCommand(
		newScheduleAddCmd(opts),
		newScheduleListCmd(opts),
		newScheduleRunCmd(opts),
		newScheduleDeleteCmd(opts),
	)

	return cmd
}

func newScheduleAddCmd(opts *RootOptions) *cobra.Command {
	flags := &scheduleAddFlags{}

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a monthly fixed expense schedule",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printScheduleError(cmd, scheduleOutputFormat(opts), &scheduleCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "schedule add does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newScheduleService(opts)
			if err != nil {
				return printScheduleError(cmd, scheduleOutputFormat(opts), err)
			}

			input, err := buildScheduleAddInput(cmd, flags)
			if err != nil {
				return printScheduleError(cmd, scheduleOutputFormat(opts), err)
			}

			schedule, err := svc.Add(cmd.Context(), input)
			if err != nil {
				return printScheduleError(cmd, scheduleOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(map[string]any{"schedule": schedule}, nil)
			return output.Print(cmd.OutOrStdout(), scheduleOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.name, "name", "", "Schedule name")
	cmd.Flags().StringVar(&flags.amount, "amount", "", "Amount in major units (e.g. 1200.00)")
	cmd.Flags().StringVar(&flags.currency, "currency", defaultEntryCurrency, "ISO currency code (e.g. USD)")
	cmd.Flags().IntVar(&flags.day, "day", 0, "Day of month (1-28)")
	cmd.Flags().StringVar(&flags.startMonthRaw, "start-month", "", "Start month in YYYY-MM")
	cmd.Flags().StringVar(&flags.endMonthRaw, "end-month", "", "Optional end month in YYYY-MM")
	cmd.Flags().StringVar(&flags.categoryIDRaw, "category-id", "", "Optional category ID")
	cmd.Flags().StringVar(&flags.note, "note", "", "Optional note")

	return cmd
}

func newScheduleListCmd(opts *RootOptions) *cobra.Command {
	flags := &scheduleListFlags{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List schedules",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printScheduleError(cmd, scheduleOutputFormat(opts), &scheduleCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "schedule list does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newScheduleService(opts)
			if err != nil {
				return printScheduleError(cmd, scheduleOutputFormat(opts), err)
			}

			schedules, err := svc.List(cmd.Context(), flags.includeDeleted)
			if err != nil {
				return printScheduleError(cmd, scheduleOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(map[string]any{
				"schedules": schedules,
				"count":     len(schedules),
			}, nil)
			return output.Print(cmd.OutOrStdout(), scheduleOutputFormat(opts), env)
		},
	}

	cmd.Flags().BoolVar(&flags.includeDeleted, "include-deleted", false, "Include soft-deleted schedules")

	return cmd
}

func newScheduleRunCmd(opts *RootOptions) *cobra.Command {
	flags := &scheduleRunFlags{}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute scheduled payments through a date",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printScheduleError(cmd, scheduleOutputFormat(opts), &scheduleCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "schedule run does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newScheduleService(opts)
			if err != nil {
				return printScheduleError(cmd, scheduleOutputFormat(opts), err)
			}

			input, err := buildScheduleRunInput(cmd, flags)
			if err != nil {
				return printScheduleError(cmd, scheduleOutputFormat(opts), err)
			}

			result, err := svc.Run(cmd.Context(), input)
			if err != nil {
				return printScheduleError(cmd, scheduleOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(map[string]any{"run": result}, nil)
			return output.Print(cmd.OutOrStdout(), scheduleOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.throughDateRaw, "through-date", "", "Run through date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.scheduleIDRaw, "schedule-id", "", "Optional schedule ID filter")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Calculate without creating entries")

	return cmd
}

func newScheduleDeleteCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft-delete a schedule",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return printScheduleError(cmd, scheduleOutputFormat(opts), &scheduleCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "delete requires exactly one argument: <id>",
					Details: map[string]any{"required_args": []string{"id"}},
				})
			}

			svc, err := newScheduleService(opts)
			if err != nil {
				return printScheduleError(cmd, scheduleOutputFormat(opts), err)
			}

			id, err := parsePositiveInt64(args[0], "id")
			if err != nil {
				return printScheduleError(cmd, scheduleOutputFormat(opts), err)
			}

			deleted, err := svc.Delete(cmd.Context(), id)
			if err != nil {
				return printScheduleError(cmd, scheduleOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(map[string]any{"deleted": deleted}, nil)
			return output.Print(cmd.OutOrStdout(), scheduleOutputFormat(opts), env)
		},
	}
}

func newScheduleService(opts *RootOptions) (*service.ScheduleService, error) {
	if opts == nil || opts.db == nil {
		return nil, &scheduleCLIError{
			Code:    "DB_ERROR",
			Message: "database operation failed",
			Details: map[string]any{"reason": "database connection unavailable"},
		}
	}

	scheduleRepo := sqlitestore.NewScheduleRepo(opts.db)
	entryRepo := sqlitestore.NewEntryRepo(opts.db)
	entryService, err := service.NewEntryService(entryRepo)
	if err != nil {
		return nil, fmt.Errorf("entry service init: %w", err)
	}

	svc, err := service.NewScheduleService(scheduleRepo, entryService)
	if err != nil {
		return nil, fmt.Errorf("schedule service init: %w", err)
	}

	return svc, nil
}

func buildScheduleAddInput(cmd *cobra.Command, flags *scheduleAddFlags) (domain.ScheduledPaymentAddInput, error) {
	if flags == nil {
		return domain.ScheduledPaymentAddInput{}, &scheduleCLIError{
			Code:    "INTERNAL_ERROR",
			Message: "schedule add flags unavailable",
			Details: map[string]any{},
		}
	}

	if strings.TrimSpace(flags.name) == "" {
		return domain.ScheduledPaymentAddInput{}, &scheduleCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "name is required",
			Details: map[string]any{"field": "name"},
		}
	}
	if cmd != nil && !cmd.Flags().Changed("amount") {
		return domain.ScheduledPaymentAddInput{}, &scheduleCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "amount is required",
			Details: map[string]any{"field": "amount"},
		}
	}
	if cmd != nil && !cmd.Flags().Changed("day") {
		return domain.ScheduledPaymentAddInput{}, &scheduleCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "day is required",
			Details: map[string]any{"field": "day"},
		}
	}

	startMonth, err := normalizeScheduleMonthKey(flags.startMonthRaw, "start-month")
	if err != nil {
		return domain.ScheduledPaymentAddInput{}, err
	}

	var endMonth *string
	if cmd != nil && cmd.Flags().Changed("end-month") {
		normalizedEndMonth, err := normalizeScheduleMonthKey(flags.endMonthRaw, "end-month")
		if err != nil {
			return domain.ScheduledPaymentAddInput{}, err
		}
		endMonth = &normalizedEndMonth
	}

	var categoryID *int64
	if cmd != nil && cmd.Flags().Changed("category-id") {
		id, err := parsePositiveInt64(flags.categoryIDRaw, "category-id")
		if err != nil {
			return domain.ScheduledPaymentAddInput{}, err
		}
		categoryID = &id
	}

	amountMinor, err := domain.ParseMajorAmountToMinor(flags.amount, flags.currency)
	if err != nil {
		return domain.ScheduledPaymentAddInput{}, err
	}

	return domain.ScheduledPaymentAddInput{
		Name:          flags.name,
		AmountMinor:   amountMinor,
		CurrencyCode:  flags.currency,
		DayOfMonth:    flags.day,
		StartMonthKey: startMonth,
		EndMonthKey:   endMonth,
		CategoryID:    categoryID,
		Note:          flags.note,
	}, nil
}

func buildScheduleRunInput(cmd *cobra.Command, flags *scheduleRunFlags) (domain.ScheduledPaymentRunInput, error) {
	if flags == nil {
		return domain.ScheduledPaymentRunInput{}, &scheduleCLIError{
			Code:    "INTERNAL_ERROR",
			Message: "schedule run flags unavailable",
			Details: map[string]any{},
		}
	}

	if strings.TrimSpace(flags.throughDateRaw) == "" {
		return domain.ScheduledPaymentRunInput{}, &scheduleCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "through-date is required",
			Details: map[string]any{"field": "through-date"},
		}
	}

	var scheduleID *int64
	if cmd != nil && cmd.Flags().Changed("schedule-id") {
		id, err := parsePositiveInt64(flags.scheduleIDRaw, "schedule-id")
		if err != nil {
			return domain.ScheduledPaymentRunInput{}, err
		}
		scheduleID = &id
	}

	return domain.ScheduledPaymentRunInput{
		ThroughDateUTC: flags.throughDateRaw,
		ScheduleID:     scheduleID,
		DryRun:         flags.dryRun,
	}, nil
}

func normalizeScheduleMonthKey(raw string, field string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", &scheduleCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: fmt.Sprintf("%s is required", field),
			Details: map[string]any{"field": field},
		}
	}

	normalized, err := domain.NormalizeMonthKey(value)
	if err != nil {
		return "", &scheduleCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: fmt.Sprintf("%s must use YYYY-MM", field),
			Details: map[string]any{"field": field, "value": raw},
		}
	}

	return normalized, nil
}

func scheduleOutputFormat(opts *RootOptions) string {
	if opts == nil {
		return output.FormatHuman
	}
	return opts.Output
}

func printScheduleError(cmd *cobra.Command, format string, err error) error {
	if cmd == nil {
		return fmt.Errorf("nil command")
	}

	if err == nil {
		env := output.NewErrorEnvelope("INTERNAL_ERROR", "unexpected internal failure", map[string]any{}, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	var cliErr *scheduleCLIError
	if errors.As(err, &cliErr) {
		env := output.NewErrorEnvelope(cliErr.Code, cliErr.Message, cliErr.Details, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	env := output.NewErrorEnvelope(codeFromScheduleError(err), messageFromScheduleError(err), map[string]any{"reason": err.Error()}, nil)
	return output.Print(cmd.OutOrStdout(), format, env)
}

func codeFromScheduleError(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidCurrencyCode):
		return "INVALID_CURRENCY_CODE"
	case errors.Is(err, domain.ErrInvalidScheduleID),
		errors.Is(err, domain.ErrScheduleNameRequired),
		errors.Is(err, domain.ErrScheduleNameTooLong),
		errors.Is(err, domain.ErrInvalidScheduleDayOfMonth),
		errors.Is(err, domain.ErrScheduleEndMonthBeforeStart),
		errors.Is(err, domain.ErrInvalidScheduleThroughDate),
		errors.Is(err, domain.ErrInvalidMonthKey),
		errors.Is(err, domain.ErrInvalidAmount),
		errors.Is(err, domain.ErrInvalidAmountPrecision),
		errors.Is(err, domain.ErrAmountOverflow),
		errors.Is(err, domain.ErrInvalidAmountMinor),
		errors.Is(err, domain.ErrInvalidCategoryID):
		return "INVALID_ARGUMENT"
	case errors.Is(err, domain.ErrScheduleNotFound),
		errors.Is(err, domain.ErrCategoryNotFound):
		return "NOT_FOUND"
	default:
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "unique constraint") || strings.Contains(message, "constraint failed") {
			return "CONFLICT"
		}
		return "DB_ERROR"
	}
}

func messageFromScheduleError(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidCurrencyCode):
		return "currency must be a 3-letter ISO code"
	case errors.Is(err, domain.ErrInvalidScheduleID):
		return "schedule id must be a positive integer"
	case errors.Is(err, domain.ErrScheduleNameRequired):
		return "name is required"
	case errors.Is(err, domain.ErrScheduleNameTooLong):
		return fmt.Sprintf("name must be at most %d characters", domain.ScheduleNameMaxLength)
	case errors.Is(err, domain.ErrInvalidScheduleDayOfMonth):
		return "day must be between 1 and 28"
	case errors.Is(err, domain.ErrInvalidMonthKey):
		return "month must use YYYY-MM"
	case errors.Is(err, domain.ErrScheduleEndMonthBeforeStart):
		return "end-month must be greater than or equal to start-month"
	case errors.Is(err, domain.ErrInvalidAmount):
		return "amount must be a valid decimal number"
	case errors.Is(err, domain.ErrInvalidAmountPrecision):
		return "amount has too many decimal places for currency"
	case errors.Is(err, domain.ErrAmountOverflow):
		return "amount is too large"
	case errors.Is(err, domain.ErrInvalidAmountMinor):
		return "amount must be greater than zero"
	case errors.Is(err, domain.ErrInvalidScheduleThroughDate):
		return "through-date must be RFC3339 or YYYY-MM-DD"
	case errors.Is(err, domain.ErrInvalidCategoryID):
		return "category-id must be a positive integer"
	case errors.Is(err, domain.ErrScheduleNotFound):
		return "schedule not found"
	case errors.Is(err, domain.ErrCategoryNotFound):
		return "category not found"
	default:
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "unique constraint") || strings.Contains(message, "constraint failed") {
			return "conflict while processing schedule"
		}
		return "database operation failed"
	}
}
