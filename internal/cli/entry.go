package cli

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"budgetto/internal/cli/output"
	"budgetto/internal/domain"
	"budgetto/internal/service"
	sqlitestore "budgetto/internal/store/sqlite"
	"github.com/spf13/cobra"
)

const (
	defaultEntryCurrency = "USD"
)

type entryAddFlags struct {
	entryType     string
	amountMinor   int64
	currency      string
	dateRaw       string
	categoryIDRaw string
	labelIDRaw    []string
	note          string
}

type entryListFlags struct {
	entryType     string
	categoryIDRaw string
	fromRaw       string
	toRaw         string
	labelIDRaw    []string
	labelMode     string
}

type entryUpdateFlags struct {
	entryType     string
	amountMinor   int64
	currency      string
	dateRaw       string
	categoryIDRaw string
	clearCategory bool
	labelIDRaw    []string
	clearLabels   bool
	note          string
	clearNote     bool
}

type entryCLIError struct {
	Code    string
	Message string
	Details any
}

func (e *entryCLIError) Error() string {
	if e == nil {
		return "entry command error"
	}
	return e.Message
}

func NewEntryCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "entry",
		Short: "Manage entries",
	}

	cmd.AddCommand(
		newEntryAddCmd(opts),
		newEntryUpdateCmd(opts),
		newEntryListCmd(opts),
		newEntryDeleteCmd(opts),
	)

	return cmd
}

func newEntryUpdateCmd(opts *RootOptions) *cobra.Command {
	flags := &entryUpdateFlags{}

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a transaction entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return printEntryError(cmd, entryOutputFormat(opts), &entryCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "update requires exactly one argument: <id>",
					Details: map[string]any{"required_args": []string{"id"}},
				})
			}

			svc, err := newEntryService(opts)
			if err != nil {
				return printEntryError(cmd, entryOutputFormat(opts), err)
			}

			id, err := parsePositiveInt64(args[0], "id")
			if err != nil {
				return printEntryError(cmd, entryOutputFormat(opts), err)
			}

			input, err := buildEntryUpdateInput(cmd, id, flags)
			if err != nil {
				return printEntryError(cmd, entryOutputFormat(opts), err)
			}

			result, err := svc.UpdateWithWarnings(cmd.Context(), input)
			if err != nil {
				return printEntryError(cmd, entryOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(
				map[string]any{"entry": result.Entry},
				toOutputWarnings(result.Warnings),
			)
			return output.Print(cmd.OutOrStdout(), entryOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.entryType, "type", "", "Optional entry type: income|expense")
	cmd.Flags().Int64Var(&flags.amountMinor, "amount-minor", 0, "Optional amount in minor units (must be > 0)")
	cmd.Flags().StringVar(&flags.currency, "currency", "", "Optional ISO currency code (e.g. USD)")
	cmd.Flags().StringVar(&flags.dateRaw, "date", "", "Optional transaction date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.categoryIDRaw, "category-id", "", "Optional category ID to set")
	cmd.Flags().BoolVar(&flags.clearCategory, "clear-category", false, "Clear category (make entry orphan)")
	cmd.Flags().StringArrayVar(&flags.labelIDRaw, "label-id", nil, "Optional label IDs to replace current labels")
	cmd.Flags().BoolVar(&flags.clearLabels, "clear-labels", false, "Clear all labels")
	cmd.Flags().StringVar(&flags.note, "note", "", "Optional note value to set")
	cmd.Flags().BoolVar(&flags.clearNote, "clear-note", false, "Clear note")

	return cmd
}

func newEntryAddCmd(opts *RootOptions) *cobra.Command {
	flags := &entryAddFlags{}

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a transaction entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printEntryError(cmd, entryOutputFormat(opts), &entryCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "entry add does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newEntryService(opts)
			if err != nil {
				return printEntryError(cmd, entryOutputFormat(opts), err)
			}

			input, err := buildEntryAddInput(cmd, flags)
			if err != nil {
				return printEntryError(cmd, entryOutputFormat(opts), err)
			}

			result, err := svc.AddWithWarnings(cmd.Context(), input)
			if err != nil {
				return printEntryError(cmd, entryOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(
				map[string]any{"entry": result.Entry},
				toOutputWarnings(result.Warnings),
			)
			return output.Print(cmd.OutOrStdout(), entryOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.entryType, "type", "", "Entry type: income|expense")
	cmd.Flags().Int64Var(&flags.amountMinor, "amount-minor", 0, "Amount in minor units (must be > 0)")
	cmd.Flags().StringVar(&flags.currency, "currency", defaultEntryCurrency, "ISO currency code (e.g. USD)")
	cmd.Flags().StringVar(&flags.dateRaw, "date", "", "Transaction date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.categoryIDRaw, "category-id", "", "Optional category ID")
	cmd.Flags().StringArrayVar(&flags.labelIDRaw, "label-id", nil, "Optional label ID (repeatable)")
	cmd.Flags().StringVar(&flags.note, "note", "", "Optional note")

	return cmd
}

func newEntryListCmd(opts *RootOptions) *cobra.Command {
	flags := &entryListFlags{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printEntryError(cmd, entryOutputFormat(opts), &entryCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "entry list does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newEntryService(opts)
			if err != nil {
				return printEntryError(cmd, entryOutputFormat(opts), err)
			}

			filter, err := buildEntryListFilter(flags)
			if err != nil {
				return printEntryError(cmd, entryOutputFormat(opts), err)
			}

			entries, err := svc.List(cmd.Context(), filter)
			if err != nil {
				return printEntryError(cmd, entryOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(map[string]any{
				"entries": entries,
				"count":   len(entries),
			}, nil)
			return output.Print(cmd.OutOrStdout(), entryOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.entryType, "type", "", "Filter by entry type: income|expense")
	cmd.Flags().StringVar(&flags.categoryIDRaw, "category-id", "", "Filter by category ID")
	cmd.Flags().StringVar(&flags.fromRaw, "from", "", "Filter start date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.toRaw, "to", "", "Filter end date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringArrayVar(&flags.labelIDRaw, "label-id", nil, "Filter by label ID (repeatable)")
	cmd.Flags().StringVar(&flags.labelMode, "label-mode", "any", "Label filter mode: any|all|none")

	return cmd
}

func newEntryDeleteCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft-delete an entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return printEntryError(cmd, entryOutputFormat(opts), &entryCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "delete requires exactly one argument: <id>",
					Details: map[string]any{"required_args": []string{"id"}},
				})
			}

			svc, err := newEntryService(opts)
			if err != nil {
				return printEntryError(cmd, entryOutputFormat(opts), err)
			}

			id, err := parsePositiveInt64(args[0], "id")
			if err != nil {
				return printEntryError(cmd, entryOutputFormat(opts), err)
			}

			deleted, err := svc.Delete(cmd.Context(), id)
			if err != nil {
				return printEntryError(cmd, entryOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(map[string]any{"deleted": deleted}, nil)
			return output.Print(cmd.OutOrStdout(), entryOutputFormat(opts), env)
		},
	}
}

func newEntryService(opts *RootOptions) (*service.EntryService, error) {
	if opts == nil || opts.db == nil {
		return nil, &entryCLIError{
			Code:    "DB_ERROR",
			Message: "database operation failed",
			Details: map[string]any{"reason": "database connection unavailable"},
		}
	}

	entryRepo := sqlitestore.NewEntryRepo(opts.db)
	capRepo := sqlitestore.NewCapRepo(opts.db)

	svc, err := service.NewEntryService(entryRepo, service.WithEntryCapLookup(capRepo))
	if err != nil {
		return nil, fmt.Errorf("entry service init: %w", err)
	}

	return svc, nil
}

func toOutputWarnings(warnings []domain.Warning) []output.WarningPayload {
	if len(warnings) == 0 {
		return []output.WarningPayload{}
	}

	out := make([]output.WarningPayload, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, output.WarningPayload{
			Code:    warning.Code,
			Message: warning.Message,
			Details: warning.Details,
		})
	}
	return out
}

func buildEntryAddInput(cmd *cobra.Command, flags *entryAddFlags) (domain.EntryAddInput, error) {
	if flags == nil {
		return domain.EntryAddInput{}, &entryCLIError{Code: "INTERNAL_ERROR", Message: "entry add flags unavailable", Details: map[string]any{}}
	}

	if strings.TrimSpace(flags.entryType) == "" {
		return domain.EntryAddInput{}, &entryCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "type is required",
			Details: map[string]any{"field": "type"},
		}
	}
	if strings.TrimSpace(flags.dateRaw) == "" {
		return domain.EntryAddInput{}, &entryCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "date is required",
			Details: map[string]any{"field": "date"},
		}
	}

	labelIDs, err := parsePositiveInt64List(flags.labelIDRaw, "label-id")
	if err != nil {
		return domain.EntryAddInput{}, err
	}

	var categoryID *int64
	if cmd != nil && cmd.Flags().Changed("category-id") {
		id, err := parsePositiveInt64(flags.categoryIDRaw, "category-id")
		if err != nil {
			return domain.EntryAddInput{}, err
		}
		categoryID = &id
	}

	return domain.EntryAddInput{
		Type:               flags.entryType,
		AmountMinor:        flags.amountMinor,
		CurrencyCode:       flags.currency,
		TransactionDateUTC: flags.dateRaw,
		CategoryID:         categoryID,
		LabelIDs:           labelIDs,
		Note:               flags.note,
	}, nil
}

func buildEntryUpdateInput(cmd *cobra.Command, id int64, flags *entryUpdateFlags) (domain.EntryUpdateInput, error) {
	if flags == nil {
		return domain.EntryUpdateInput{}, &entryCLIError{
			Code:    "INTERNAL_ERROR",
			Message: "entry update flags unavailable",
			Details: map[string]any{},
		}
	}

	if cmd != nil && cmd.Flags().Changed("clear-category") && cmd.Flags().Changed("category-id") {
		return domain.EntryUpdateInput{}, &entryCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "clear-category cannot be used with category-id",
			Details: map[string]any{"fields": []string{"clear-category", "category-id"}},
		}
	}
	if cmd != nil && cmd.Flags().Changed("clear-labels") && cmd.Flags().Changed("label-id") {
		return domain.EntryUpdateInput{}, &entryCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "clear-labels cannot be used with label-id",
			Details: map[string]any{"fields": []string{"clear-labels", "label-id"}},
		}
	}
	if cmd != nil && cmd.Flags().Changed("clear-note") && cmd.Flags().Changed("note") {
		return domain.EntryUpdateInput{}, &entryCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "clear-note cannot be used with note",
			Details: map[string]any{"fields": []string{"clear-note", "note"}},
		}
	}

	input := domain.EntryUpdateInput{ID: id}
	changed := false

	if cmd != nil && cmd.Flags().Changed("type") {
		changed = true
		value := flags.entryType
		input.Type = &value
	}
	if cmd != nil && cmd.Flags().Changed("amount-minor") {
		changed = true
		value := flags.amountMinor
		input.AmountMinor = &value
	}
	if cmd != nil && cmd.Flags().Changed("currency") {
		changed = true
		value := flags.currency
		input.CurrencyCode = &value
	}
	if cmd != nil && cmd.Flags().Changed("date") {
		changed = true
		value := flags.dateRaw
		input.TransactionDateUTC = &value
	}
	if cmd != nil && cmd.Flags().Changed("clear-category") {
		changed = true
		input.SetCategory = true
		input.CategoryID = nil
	}
	if cmd != nil && cmd.Flags().Changed("category-id") {
		changed = true
		categoryID, err := parsePositiveInt64(flags.categoryIDRaw, "category-id")
		if err != nil {
			return domain.EntryUpdateInput{}, err
		}
		input.SetCategory = true
		input.CategoryID = &categoryID
	}
	if cmd != nil && cmd.Flags().Changed("clear-labels") {
		changed = true
		input.SetLabelIDs = true
		input.LabelIDs = []int64{}
	}
	if cmd != nil && cmd.Flags().Changed("label-id") {
		changed = true
		labelIDs, err := parsePositiveInt64List(flags.labelIDRaw, "label-id")
		if err != nil {
			return domain.EntryUpdateInput{}, err
		}
		input.SetLabelIDs = true
		input.LabelIDs = labelIDs
	}
	if cmd != nil && cmd.Flags().Changed("clear-note") {
		changed = true
		input.SetNote = true
		input.Note = nil
	}
	if cmd != nil && cmd.Flags().Changed("note") {
		changed = true
		value := flags.note
		input.SetNote = true
		input.Note = &value
	}

	if !changed {
		return domain.EntryUpdateInput{}, &entryCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "at least one update field is required",
			Details: map[string]any{
				"supported_fields": []string{
					"type",
					"amount-minor",
					"currency",
					"date",
					"category-id|clear-category",
					"label-id|clear-labels",
					"note|clear-note",
				},
			},
		}
	}

	return input, nil
}

func buildEntryListFilter(flags *entryListFlags) (domain.EntryListFilter, error) {
	if flags == nil {
		return domain.EntryListFilter{}, &entryCLIError{Code: "INTERNAL_ERROR", Message: "entry list flags unavailable", Details: map[string]any{}}
	}

	var categoryID *int64
	if strings.TrimSpace(flags.categoryIDRaw) != "" {
		id, err := parsePositiveInt64(flags.categoryIDRaw, "category-id")
		if err != nil {
			return domain.EntryListFilter{}, err
		}
		categoryID = &id
	}

	labelIDs, err := parsePositiveInt64List(flags.labelIDRaw, "label-id")
	if err != nil {
		return domain.EntryListFilter{}, err
	}

	fromUTC, err := normalizeListDateBound(flags.fromRaw, false)
	if err != nil {
		return domain.EntryListFilter{}, &entryCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "from must be RFC3339 or YYYY-MM-DD",
			Details: map[string]any{"field": "from", "value": flags.fromRaw},
		}
	}
	toUTC, err := normalizeListDateBound(flags.toRaw, true)
	if err != nil {
		return domain.EntryListFilter{}, &entryCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "to must be RFC3339 or YYYY-MM-DD",
			Details: map[string]any{"field": "to", "value": flags.toRaw},
		}
	}

	return domain.EntryListFilter{
		Type:        flags.entryType,
		CategoryID:  categoryID,
		DateFromUTC: fromUTC,
		DateToUTC:   toUTC,
		LabelIDs:    labelIDs,
		LabelMode:   flags.labelMode,
	}, nil
}

func parsePositiveInt64(raw, field string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, &entryCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: fmt.Sprintf("%s is required", field),
			Details: map[string]any{"field": field},
		}
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, &entryCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: fmt.Sprintf("%s must be a positive integer", field),
			Details: map[string]any{"field": field, "value": raw},
		}
	}

	return parsed, nil
}

func parsePositiveInt64List(rawIDs []string, field string) ([]int64, error) {
	if len(rawIDs) == 0 {
		return nil, nil
	}

	seen := make(map[int64]struct{}, len(rawIDs))
	ids := make([]int64, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, err := parsePositiveInt64(raw, field)
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

func normalizeListDateBound(raw string, endOfDay bool) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}

	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t.UTC().Format(time.RFC3339Nano), nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC().Format(time.RFC3339Nano), nil
	}
	if t, err := time.Parse("2006-01-02", value); err == nil {
		if endOfDay {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		return t.UTC().Format(time.RFC3339Nano), nil
	}

	return "", fmt.Errorf("invalid date")
}

func entryOutputFormat(opts *RootOptions) string {
	if opts == nil {
		return output.FormatHuman
	}
	return opts.Output
}

func printEntryError(cmd *cobra.Command, format string, err error) error {
	if cmd == nil {
		return fmt.Errorf("nil command")
	}

	if err == nil {
		env := output.NewErrorEnvelope("INTERNAL_ERROR", "unexpected internal failure", map[string]any{}, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	var cliErr *entryCLIError
	if errors.As(err, &cliErr) {
		env := output.NewErrorEnvelope(cliErr.Code, cliErr.Message, cliErr.Details, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	env := output.NewErrorEnvelope(codeFromEntryError(err), messageFromEntryError(err), map[string]any{"reason": err.Error()}, nil)
	return output.Print(cmd.OutOrStdout(), format, env)
}

func codeFromEntryError(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidCurrencyCode):
		return "INVALID_CURRENCY_CODE"
	case errors.Is(err, domain.ErrInvalidDateRange):
		return "INVALID_DATE_RANGE"
	case errors.Is(err, domain.ErrInvalidEntryType),
		errors.Is(err, domain.ErrInvalidAmountMinor),
		errors.Is(err, domain.ErrInvalidTransactionDate),
		errors.Is(err, domain.ErrInvalidEntryID),
		errors.Is(err, domain.ErrNoEntryUpdateFields),
		errors.Is(err, domain.ErrInvalidCategoryID),
		errors.Is(err, domain.ErrInvalidLabelID),
		errors.Is(err, domain.ErrInvalidLabelMode):
		return "INVALID_ARGUMENT"
	case errors.Is(err, domain.ErrCategoryNotFound),
		errors.Is(err, domain.ErrLabelNotFound),
		errors.Is(err, domain.ErrEntryNotFound):
		return "NOT_FOUND"
	default:
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed") {
			return "CONFLICT"
		}
		return "DB_ERROR"
	}
}

func messageFromEntryError(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidCurrencyCode):
		return "currency must be a 3-letter ISO code"
	case errors.Is(err, domain.ErrInvalidDateRange):
		return "from must be less than or equal to to"
	case errors.Is(err, domain.ErrInvalidEntryType):
		return "type must be one of: income|expense"
	case errors.Is(err, domain.ErrInvalidAmountMinor):
		return "amount-minor must be greater than zero"
	case errors.Is(err, domain.ErrInvalidTransactionDate):
		return "date/from/to must be RFC3339 or YYYY-MM-DD"
	case errors.Is(err, domain.ErrInvalidEntryID):
		return "id must be a positive integer"
	case errors.Is(err, domain.ErrNoEntryUpdateFields):
		return "at least one update field is required"
	case errors.Is(err, domain.ErrInvalidCategoryID):
		return "category-id must be a positive integer"
	case errors.Is(err, domain.ErrInvalidLabelID):
		return "label-id must be a positive integer"
	case errors.Is(err, domain.ErrInvalidLabelMode):
		return "label-mode must be one of: any|all|none"
	case errors.Is(err, domain.ErrCategoryNotFound):
		return "category not found"
	case errors.Is(err, domain.ErrLabelNotFound):
		return "label not found"
	case errors.Is(err, domain.ErrEntryNotFound):
		return "entry not found"
	default:
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed") {
			return "conflict while processing entry"
		}
		return "database operation failed"
	}
}
