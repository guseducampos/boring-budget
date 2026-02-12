package cli

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"boring-budget/internal/cli/output"
	"boring-budget/internal/domain"
	"boring-budget/internal/fx"
	"boring-budget/internal/service"
	sqlitestore "boring-budget/internal/store/sqlite"
	"github.com/spf13/cobra"
)

const (
	reportScopeRange     = "range"
	reportScopeMonthly   = "monthly"
	reportScopeBimonthly = "bimonthly"
	reportScopeQuarterly = "quarterly"

	reportGroupByDay   = "day"
	reportGroupByWeek  = "week"
	reportGroupByMonth = "month"
)

type reportCommonFlags struct {
	groupBy       string
	categoryIDRaw string
	labelIDRaw    []string
	labelMode     string
	convertTo     string
}

type reportRangeFlags struct {
	reportCommonFlags
	fromRaw string
	toRaw   string
}

type reportPresetFlags struct {
	reportCommonFlags
	monthRaw string
}

type reportCLIError struct {
	Code    string
	Message string
	Details any
}

func (e *reportCLIError) Error() string {
	if e == nil {
		return "report command error"
	}
	return e.Message
}

type reportPeriodInput struct {
	Scope    string
	MonthKey string
	FromUTC  string
	ToUTC    string
}

func NewReportCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate earnings and spending reports",
	}

	cmd.AddCommand(
		newReportRangeCmd(opts),
		newReportMonthlyCmd(opts),
		newReportBimonthlyCmd(opts),
		newReportQuarterlyCmd(opts),
	)

	return cmd
}

func newReportRangeCmd(opts *RootOptions) *cobra.Command {
	flags := &reportRangeFlags{}

	cmd := &cobra.Command{
		Use:   "range",
		Short: "Generate a report for a custom range",
		RunE: func(cmd *cobra.Command, args []string) error {
			period, err := buildRangeReportPeriod(flags)
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}
			return runReportCommand(cmd, args, opts, flags.reportCommonFlags, period)
		},
	}

	bindReportCommonFlags(cmd, &flags.reportCommonFlags)
	cmd.Flags().StringVar(&flags.fromRaw, "from", "", "Filter start date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.toRaw, "to", "", "Filter end date (RFC3339 or YYYY-MM-DD)")

	return cmd
}

func newReportMonthlyCmd(opts *RootOptions) *cobra.Command {
	flags := &reportPresetFlags{}

	cmd := &cobra.Command{
		Use:   "monthly",
		Short: "Generate a report for one month",
		RunE: func(cmd *cobra.Command, args []string) error {
			period, err := buildPresetReportPeriod(flags.monthRaw, reportScopeMonthly)
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}
			return runReportCommand(cmd, args, opts, flags.reportCommonFlags, period)
		},
	}

	bindReportCommonFlags(cmd, &flags.reportCommonFlags)
	cmd.Flags().StringVar(&flags.monthRaw, "month", "", "Target month in YYYY-MM")

	return cmd
}

func newReportBimonthlyCmd(opts *RootOptions) *cobra.Command {
	flags := &reportPresetFlags{}

	cmd := &cobra.Command{
		Use:   "bimonthly",
		Short: "Generate a report for two months starting from --month",
		RunE: func(cmd *cobra.Command, args []string) error {
			period, err := buildPresetReportPeriod(flags.monthRaw, reportScopeBimonthly)
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}
			return runReportCommand(cmd, args, opts, flags.reportCommonFlags, period)
		},
	}

	bindReportCommonFlags(cmd, &flags.reportCommonFlags)
	cmd.Flags().StringVar(&flags.monthRaw, "month", "", "Anchor month in YYYY-MM")

	return cmd
}

func newReportQuarterlyCmd(opts *RootOptions) *cobra.Command {
	flags := &reportPresetFlags{}

	cmd := &cobra.Command{
		Use:   "quarterly",
		Short: "Generate a report for three months starting from --month",
		RunE: func(cmd *cobra.Command, args []string) error {
			period, err := buildPresetReportPeriod(flags.monthRaw, reportScopeQuarterly)
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}
			return runReportCommand(cmd, args, opts, flags.reportCommonFlags, period)
		},
	}

	bindReportCommonFlags(cmd, &flags.reportCommonFlags)
	cmd.Flags().StringVar(&flags.monthRaw, "month", "", "Anchor month in YYYY-MM")

	return cmd
}

func bindReportCommonFlags(cmd *cobra.Command, flags *reportCommonFlags) {
	if cmd == nil || flags == nil {
		return
	}

	cmd.Flags().StringVar(&flags.groupBy, "group-by", reportGroupByMonth, "Grouping: day|week|month")
	cmd.Flags().StringVar(&flags.categoryIDRaw, "category-id", "", "Filter by category ID")
	cmd.Flags().StringArrayVar(&flags.labelIDRaw, "label-id", nil, "Filter by label ID (repeatable)")
	cmd.Flags().StringVar(&flags.labelMode, "label-mode", "any", "Label filter mode: any|all|none")
	cmd.Flags().StringVar(&flags.convertTo, "convert-to", "", "Optional target currency (ISO code) for converted totals")
}

func runReportCommand(cmd *cobra.Command, args []string, opts *RootOptions, flags reportCommonFlags, period reportPeriodInput) error {
	if len(args) != 0 {
		return printReportError(cmd, reportOutputFormat(opts), &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: fmt.Sprintf("report %s does not accept positional arguments", period.Scope),
			Details: map[string]any{"args": args},
		})
	}

	reportSvc, err := newReportService(opts)
	if err != nil {
		return printReportError(cmd, reportOutputFormat(opts), err)
	}

	req, err := buildReportRequest(flags, period)
	if err != nil {
		return printReportError(cmd, reportOutputFormat(opts), err)
	}

	result, err := reportSvc.Generate(cmd.Context(), req)
	if err != nil {
		return printReportError(cmd, reportOutputFormat(opts), err)
	}

	env := output.NewSuccessEnvelope(result.Report, toOutputWarnings(result.Warnings))
	return output.Print(cmd.OutOrStdout(), reportOutputFormat(opts), env)
}

func newReportService(opts *RootOptions) (*service.ReportService, error) {
	if opts == nil || opts.db == nil {
		return nil, &reportCLIError{
			Code:    "DB_ERROR",
			Message: "database operation failed",
			Details: map[string]any{"reason": "database connection unavailable"},
		}
	}

	entryRepo := sqlitestore.NewEntryRepo(opts.db)
	entrySvc, err := service.NewEntryService(entryRepo)
	if err != nil {
		return nil, fmt.Errorf("entry service init: %w", err)
	}

	capRepo := sqlitestore.NewCapRepo(opts.db)
	capSvc, err := service.NewCapService(capRepo)
	if err != nil {
		return nil, fmt.Errorf("cap service init: %w", err)
	}

	categoryRepo := sqlitestore.NewCategoryRepo(opts.db)
	settingsRepo := sqlitestore.NewSettingsRepo(opts.db)
	reportOptions := []service.ReportServiceOption{
		service.WithReportSettingsReader(settingsRepo),
		service.WithReportCategoryReader(categoryRepo),
	}

	reportSvc, err := service.NewReportService(entrySvc, capSvc, reportOptions...)
	if err != nil {
		return nil, fmt.Errorf("report service init: %w", err)
	}

	fxRepo := sqlitestore.NewFXRepo(opts.db)
	fxClient := fx.NewFrankfurterClient(nil)
	converter, err := fx.NewConverter(fxClient, fxRepo)
	if err == nil {
		reportOptions = append(reportOptions, service.WithReportFXConverter(converter))
		reportSvc, err = service.NewReportService(entrySvc, capSvc, reportOptions...)
		if err != nil {
			return nil, fmt.Errorf("report service init with fx: %w", err)
		}
	}

	return reportSvc, nil
}

func buildReportRequest(flags reportCommonFlags, period reportPeriodInput) (service.ReportRequest, error) {
	grouping, err := normalizeReportGroupBy(flags.groupBy)
	if err != nil {
		return service.ReportRequest{}, err
	}

	var categoryID *int64
	if strings.TrimSpace(flags.categoryIDRaw) != "" {
		id, err := parsePositiveID(flags.categoryIDRaw, "category-id")
		if err != nil {
			return service.ReportRequest{}, err
		}
		categoryID = &id
	}

	labelIDs, err := parsePositiveIDList(flags.labelIDRaw, "label-id")
	if err != nil {
		return service.ReportRequest{}, err
	}

	return service.ReportRequest{
		Period: domain.ReportPeriodInput{
			Scope:       period.Scope,
			MonthKey:    period.MonthKey,
			DateFromUTC: period.FromUTC,
			DateToUTC:   period.ToUTC,
		},
		Grouping:   grouping,
		CategoryID: categoryID,
		LabelIDs:   labelIDs,
		LabelMode:  flags.labelMode,
		ConvertTo:  flags.convertTo,
	}, nil
}

func buildRangeReportPeriod(flags *reportRangeFlags) (reportPeriodInput, error) {
	if flags == nil {
		return reportPeriodInput{}, &reportCLIError{
			Code:    "INTERNAL_ERROR",
			Message: "range report flags unavailable",
			Details: map[string]any{},
		}
	}

	fromRaw := strings.TrimSpace(flags.fromRaw)
	toRaw := strings.TrimSpace(flags.toRaw)
	if fromRaw == "" || toRaw == "" {
		return reportPeriodInput{}, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "range requires both --from and --to",
			Details: map[string]any{"required_flags": []string{"from", "to"}},
		}
	}

	fromUTC, err := normalizeListDateBound(fromRaw, false)
	if err != nil {
		return reportPeriodInput{}, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "from must be RFC3339 or YYYY-MM-DD",
			Details: map[string]any{"field": "from", "value": flags.fromRaw},
		}
	}
	toUTC, err := normalizeListDateBound(toRaw, true)
	if err != nil {
		return reportPeriodInput{}, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "to must be RFC3339 or YYYY-MM-DD",
			Details: map[string]any{"field": "to", "value": flags.toRaw},
		}
	}
	if err := domain.ValidateDateRange(fromUTC, toUTC); err != nil {
		return reportPeriodInput{}, err
	}

	return reportPeriodInput{
		Scope:   reportScopeRange,
		FromUTC: fromUTC,
		ToUTC:   toUTC,
	}, nil
}

func buildPresetReportPeriod(monthRaw, scope string) (reportPeriodInput, error) {
	monthKey, err := normalizeReportMonth(monthRaw)
	if err != nil {
		return reportPeriodInput{}, err
	}

	return reportPeriodInput{
		Scope:    scope,
		MonthKey: monthKey,
	}, nil
}

func normalizeReportMonth(raw string) (string, error) {
	month := strings.TrimSpace(raw)
	if month == "" {
		return "", &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "month is required",
			Details: map[string]any{"field": "month"},
		}
	}

	normalized, err := domain.NormalizeMonthKey(month)
	if err != nil {
		return "", &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "month must use YYYY-MM",
			Details: map[string]any{"field": "month", "value": raw},
		}
	}
	return normalized, nil
}

func normalizeReportGroupBy(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return reportGroupByMonth, nil
	}

	switch normalized {
	case reportGroupByDay, reportGroupByWeek, reportGroupByMonth:
		return normalized, nil
	default:
		return "", &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "group-by must be one of: day|week|month",
			Details: map[string]any{"field": "group-by", "value": raw},
		}
	}
}

func parsePositiveID(raw, field string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: fmt.Sprintf("%s is required", field),
			Details: map[string]any{"field": field},
		}
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: fmt.Sprintf("%s must be a positive integer", field),
			Details: map[string]any{"field": field, "value": raw},
		}
	}

	return parsed, nil
}

func parsePositiveIDList(rawIDs []string, field string) ([]int64, error) {
	if len(rawIDs) == 0 {
		return nil, nil
	}

	seen := make(map[int64]struct{}, len(rawIDs))
	ids := make([]int64, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, err := parsePositiveID(raw, field)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	return ids, nil
}

func reportOutputFormat(opts *RootOptions) string {
	if opts == nil {
		return output.FormatHuman
	}
	return opts.Output
}

func printReportError(cmd *cobra.Command, format string, err error) error {
	if cmd == nil {
		return fmt.Errorf("nil command")
	}

	if err == nil {
		env := output.NewErrorEnvelope("INTERNAL_ERROR", "unexpected internal failure", map[string]any{}, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	var cliErr *reportCLIError
	if errors.As(err, &cliErr) {
		env := output.NewErrorEnvelope(cliErr.Code, cliErr.Message, cliErr.Details, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	var entryErr *entryCLIError
	if errors.As(err, &entryErr) {
		env := output.NewErrorEnvelope(entryErr.Code, entryErr.Message, entryErr.Details, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	var capErr *capCLIError
	if errors.As(err, &capErr) {
		env := output.NewErrorEnvelope(capErr.Code, capErr.Message, capErr.Details, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	env := output.NewErrorEnvelope(codeFromReportingError(err), messageFromReportingError(err), map[string]any{"reason": err.Error()}, nil)
	return output.Print(cmd.OutOrStdout(), format, env)
}

func codeFromReportingError(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidDateRange):
		return "INVALID_DATE_RANGE"
	case errors.Is(err, domain.ErrInvalidCurrencyCode):
		return "INVALID_CURRENCY_CODE"
	case errors.Is(err, domain.ErrFXRateUnavailable):
		return "FX_RATE_UNAVAILABLE"
	case errors.Is(err, domain.ErrInvalidEntryType),
		errors.Is(err, domain.ErrInvalidAmountMinor),
		errors.Is(err, domain.ErrInvalidAmount),
		errors.Is(err, domain.ErrInvalidAmountPrecision),
		errors.Is(err, domain.ErrAmountOverflow),
		errors.Is(err, domain.ErrInvalidTransactionDate),
		errors.Is(err, domain.ErrInvalidEntryID),
		errors.Is(err, domain.ErrInvalidCategoryID),
		errors.Is(err, domain.ErrInvalidLabelID),
		errors.Is(err, domain.ErrInvalidLabelMode),
		errors.Is(err, domain.ErrInvalidMonthKey),
		errors.Is(err, domain.ErrInvalidCapAmount),
		errors.Is(err, domain.ErrInvalidReportScope),
		errors.Is(err, domain.ErrInvalidReportGrouping),
		errors.Is(err, domain.ErrInvalidReportPeriod):
		return "INVALID_ARGUMENT"
	case errors.Is(err, domain.ErrCategoryNotFound),
		errors.Is(err, domain.ErrLabelNotFound),
		errors.Is(err, domain.ErrEntryNotFound),
		errors.Is(err, domain.ErrCapNotFound),
		errors.Is(err, domain.ErrSettingsNotFound):
		return "NOT_FOUND"
	default:
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "unique constraint") || strings.Contains(message, "constraint failed") {
			return "CONFLICT"
		}
		return "DB_ERROR"
	}
}

func messageFromReportingError(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidDateRange):
		return "from must be less than or equal to to"
	case errors.Is(err, domain.ErrInvalidCurrencyCode):
		return "currency must be a 3-letter ISO code"
	case errors.Is(err, domain.ErrFXRateUnavailable):
		return "required FX rate could not be resolved"
	case errors.Is(err, domain.ErrInvalidEntryType):
		return "type must be one of: income|expense"
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
	case errors.Is(err, domain.ErrInvalidEntryID):
		return "id must be a positive integer"
	case errors.Is(err, domain.ErrInvalidCategoryID):
		return "category-id must be a positive integer"
	case errors.Is(err, domain.ErrInvalidLabelID):
		return "label-id must be a positive integer"
	case errors.Is(err, domain.ErrInvalidLabelMode):
		return "label-mode must be one of: any|all|none"
	case errors.Is(err, domain.ErrInvalidMonthKey):
		return "month must use YYYY-MM"
	case errors.Is(err, domain.ErrInvalidCapAmount):
		return "amount must be greater than zero"
	case errors.Is(err, domain.ErrInvalidReportScope):
		return "report scope is invalid"
	case errors.Is(err, domain.ErrInvalidReportGrouping):
		return "group-by must be one of: day|week|month"
	case errors.Is(err, domain.ErrInvalidReportPeriod):
		return "report period is invalid"
	case errors.Is(err, domain.ErrCategoryNotFound):
		return "category not found"
	case errors.Is(err, domain.ErrLabelNotFound):
		return "label not found"
	case errors.Is(err, domain.ErrEntryNotFound):
		return "entry not found"
	case errors.Is(err, domain.ErrCapNotFound):
		return "cap not found"
	case errors.Is(err, domain.ErrSettingsNotFound):
		return "settings not found"
	default:
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "unique constraint") || strings.Contains(message, "constraint failed") {
			return "conflict while processing report"
		}
		return "database operation failed"
	}
}
