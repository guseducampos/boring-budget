package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"boring-budget/internal/cli/output"
	"boring-budget/internal/domain"
	"boring-budget/internal/service"
	sqlitestore "boring-budget/internal/store/sqlite"
	"github.com/spf13/cobra"
)

type dataExportFlags struct {
	format              string
	file                string
	resource            string
	from                string
	to                  string
	reportScope         string
	reportMonth         string
	reportFrom          string
	reportTo            string
	reportGroupBy       string
	reportCategoryIDRaw string
	reportLabelIDRaw    []string
	reportLabelMode     string
	reportConvertTo     string
}

type dataImportFlags struct {
	format     string
	file       string
	idempotent bool
}

type dataBackupFlags struct {
	file string
}

type dataRestoreFlags struct {
	file string
}

func NewDataCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data",
		Short: "Import/export and backup/restore data",
	}

	cmd.AddCommand(
		newDataExportCmd(opts),
		newDataImportCmd(opts),
		newDataBackupCmd(opts),
		newDataRestoreCmd(opts),
	)

	return cmd
}

func newDataExportCmd(opts *RootOptions) *cobra.Command {
	flags := &dataExportFlags{}

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export entries or reports to JSON or CSV",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printReportError(cmd, reportOutputFormat(opts), invalidArgsError("data export", args))
			}
			if strings.TrimSpace(flags.format) == "" || strings.TrimSpace(flags.file) == "" {
				return printReportError(cmd, reportOutputFormat(opts), &reportCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "format and file are required",
					Details: map[string]any{"required_flags": []string{"format", "file"}},
				})
			}

			portabilitySvc, err := newPortabilityService(opts)
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			resource := normalizeDataExportResource(flags.resource)
			if resource == "" {
				return printReportError(cmd, reportOutputFormat(opts), &reportCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "resource must be one of: entries|report",
					Details: map[string]any{"field": "resource", "value": flags.resource},
				})
			}

			var data map[string]any
			var warnings []output.WarningPayload
			switch resource {
			case dataExportResourceEntries:
				fromUTC, err := normalizeListDateBound(flags.from, false)
				if err != nil {
					return printReportError(cmd, reportOutputFormat(opts), &reportCLIError{Code: "INVALID_ARGUMENT", Message: "from must be RFC3339 or YYYY-MM-DD", Details: map[string]any{"field": "from", "value": flags.from}})
				}
				toUTC, err := normalizeListDateBound(flags.to, true)
				if err != nil {
					return printReportError(cmd, reportOutputFormat(opts), &reportCLIError{Code: "INVALID_ARGUMENT", Message: "to must be RFC3339 or YYYY-MM-DD", Details: map[string]any{"field": "to", "value": flags.to}})
				}
				if err := domain.ValidateDateRange(fromUTC, toUTC); err != nil {
					return printReportError(cmd, reportOutputFormat(opts), err)
				}

				count, err := portabilitySvc.Export(cmd.Context(), flags.format, flags.file, domain.EntryListFilter{DateFromUTC: fromUTC, DateToUTC: toUTC})
				if err != nil {
					return printReportError(cmd, reportOutputFormat(opts), err)
				}

				data = map[string]any{
					"resource": resource,
					"exported": count,
					"format":   strings.ToLower(flags.format),
					"file":     flags.file,
				}
			case dataExportResourceReport:
				reportReq, err := buildDataExportReportRequest(flags)
				if err != nil {
					return printReportError(cmd, reportOutputFormat(opts), err)
				}

				result, err := portabilitySvc.ExportReport(cmd.Context(), flags.format, flags.file, reportReq)
				if err != nil {
					return printReportError(cmd, reportOutputFormat(opts), err)
				}

				reportPeriod, err := domain.BuildReportPeriod(reportReq.Period)
				if err != nil {
					return printReportError(cmd, reportOutputFormat(opts), err)
				}

				data = map[string]any{
					"resource": resource,
					"format":   strings.ToLower(flags.format),
					"file":     flags.file,
					"period":   reportPeriod,
					"grouping": reportReq.Grouping,
				}
				warnings = toOutputWarnings(result.Warnings)
			}

			env := output.NewSuccessEnvelope(data, warnings)
			return output.Print(cmd.OutOrStdout(), reportOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.resource, "resource", dataExportResourceEntries, "Export resource: entries|report")
	cmd.Flags().StringVar(&flags.format, "format", "", "Export format: json|csv")
	cmd.Flags().StringVar(&flags.file, "file", "", "Output file path")
	cmd.Flags().StringVar(&flags.from, "from", "", "Optional filter start date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.to, "to", "", "Optional filter end date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.reportScope, "report-scope", "", "Report scope for --resource report: range|monthly|bimonthly|quarterly")
	cmd.Flags().StringVar(&flags.reportMonth, "report-month", "", "Report month in YYYY-MM for preset scopes")
	cmd.Flags().StringVar(&flags.reportFrom, "report-from", "", "Report start date for range scope (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.reportTo, "report-to", "", "Report end date for range scope (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.reportGroupBy, "report-group-by", reportGroupByMonth, "Report grouping: day|week|month")
	cmd.Flags().StringVar(&flags.reportCategoryIDRaw, "report-category-id", "", "Optional report category filter")
	cmd.Flags().StringArrayVar(&flags.reportLabelIDRaw, "report-label-id", nil, "Optional report label filter (repeatable)")
	cmd.Flags().StringVar(&flags.reportLabelMode, "report-label-mode", domain.LabelFilterModeAny, "Report label filter mode: any|all|none")
	cmd.Flags().StringVar(&flags.reportConvertTo, "report-convert-to", "", "Optional report target currency (ISO code)")

	return cmd
}

func newDataImportCmd(opts *RootOptions) *cobra.Command {
	flags := &dataImportFlags{}

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import entries from JSON or CSV",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printReportError(cmd, reportOutputFormat(opts), invalidArgsError("data import", args))
			}
			if strings.TrimSpace(flags.format) == "" || strings.TrimSpace(flags.file) == "" {
				return printReportError(cmd, reportOutputFormat(opts), &reportCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "format and file are required",
					Details: map[string]any{"required_flags": []string{"format", "file"}},
				})
			}

			portabilitySvc, err := newPortabilityService(opts)
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			result, err := portabilitySvc.Import(cmd.Context(), flags.format, flags.file, flags.idempotent)
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(
				map[string]any{
					"imported":   result.Imported,
					"skipped":    result.Skipped,
					"format":     strings.ToLower(flags.format),
					"file":       flags.file,
					"idempotent": flags.idempotent,
				},
				toOutputWarnings(result.Warnings),
			)
			return output.Print(cmd.OutOrStdout(), reportOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.format, "format", "", "Import format: json|csv")
	cmd.Flags().StringVar(&flags.file, "file", "", "Input file path")
	cmd.Flags().BoolVar(&flags.idempotent, "idempotent", false, "Skip records matching existing entry fingerprints")

	return cmd
}

func newDataBackupCmd(opts *RootOptions) *cobra.Command {
	flags := &dataBackupFlags{}

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create a SQLite backup file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printReportError(cmd, reportOutputFormat(opts), invalidArgsError("data backup", args))
			}
			if strings.TrimSpace(flags.file) == "" {
				return printReportError(cmd, reportOutputFormat(opts), &reportCLIError{Code: "INVALID_ARGUMENT", Message: "file is required", Details: map[string]any{"field": "file"}})
			}

			portabilitySvc, err := newPortabilityService(opts)
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			if err := portabilitySvc.Backup(cmd.Context(), flags.file); err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(map[string]any{"backup_file": flags.file}, nil)
			return output.Print(cmd.OutOrStdout(), reportOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.file, "file", "", "Backup output file path")
	return cmd
}

func newDataRestoreCmd(opts *RootOptions) *cobra.Command {
	flags := &dataRestoreFlags{}

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore SQLite DB from a backup file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printReportError(cmd, reportOutputFormat(opts), invalidArgsError("data restore", args))
			}
			if strings.TrimSpace(flags.file) == "" {
				return printReportError(cmd, reportOutputFormat(opts), &reportCLIError{Code: "INVALID_ARGUMENT", Message: "file is required", Details: map[string]any{"field": "file"}})
			}

			if err := restoreDatabase(cmd.Context(), opts, flags.file); err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(map[string]any{"restored_from": flags.file, "db_path": opts.DBPath}, nil)
			return output.Print(cmd.OutOrStdout(), reportOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.file, "file", "", "Backup file path to restore from")
	return cmd
}

func newPortabilityService(opts *RootOptions) (*service.PortabilityService, error) {
	if opts == nil || opts.db == nil {
		return nil, &reportCLIError{Code: "DB_ERROR", Message: "database operation failed", Details: map[string]any{"reason": "database connection unavailable"}}
	}

	entryRepo := sqlitestore.NewEntryRepo(opts.db)
	capRepo := sqlitestore.NewCapRepo(opts.db)
	entrySvc, err := service.NewEntryService(entryRepo, service.WithEntryCapLookup(capRepo))
	if err != nil {
		return nil, fmt.Errorf("entry service init: %w", err)
	}

	reportSvc, err := newReportService(opts)
	if err != nil {
		return nil, fmt.Errorf("report service init: %w", err)
	}

	portabilitySvc, err := service.NewPortabilityService(entrySvc, opts.db, service.WithPortabilityReportService(reportSvc))
	if err != nil {
		return nil, fmt.Errorf("portability service init: %w", err)
	}

	return portabilitySvc, nil
}

const (
	dataExportResourceEntries = "entries"
	dataExportResourceReport  = "report"
)

func normalizeDataExportResource(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case dataExportResourceEntries:
		return dataExportResourceEntries
	case dataExportResourceReport:
		return dataExportResourceReport
	default:
		return ""
	}
}

func buildDataExportReportRequest(flags *dataExportFlags) (service.ReportRequest, error) {
	if flags == nil {
		return service.ReportRequest{}, &reportCLIError{
			Code:    "INTERNAL_ERROR",
			Message: "report export flags unavailable",
			Details: map[string]any{},
		}
	}

	reportScope := strings.TrimSpace(flags.reportScope)
	if reportScope == "" {
		return service.ReportRequest{}, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "report-scope is required for report export",
			Details: map[string]any{"required_flags": []string{"report-scope"}},
		}
	}

	scope, err := domain.NormalizeReportScope(reportScope)
	if err != nil {
		return service.ReportRequest{}, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "report-scope must be one of: range|monthly|bimonthly|quarterly",
			Details: map[string]any{"field": "report-scope", "value": flags.reportScope},
		}
	}

	var period reportPeriodInput
	switch scope {
	case reportScopeRange:
		period, err = buildDataExportReportRangePeriod(flags.reportFrom, flags.reportTo)
		if err != nil {
			return service.ReportRequest{}, err
		}
	case reportScopeMonthly, reportScopeBimonthly, reportScopeQuarterly:
		period, err = buildDataExportReportPresetPeriod(flags.reportMonth, scope)
		if err != nil {
			return service.ReportRequest{}, err
		}
	default:
		return service.ReportRequest{}, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "report-scope must be one of: range|monthly|bimonthly|quarterly",
			Details: map[string]any{"field": "report-scope", "value": flags.reportScope},
		}
	}

	return buildReportRequest(reportCommonFlags{
		groupBy:       flags.reportGroupBy,
		categoryIDRaw: flags.reportCategoryIDRaw,
		labelIDRaw:    flags.reportLabelIDRaw,
		labelMode:     flags.reportLabelMode,
		convertTo:     flags.reportConvertTo,
	}, period)
}

func buildDataExportReportRangePeriod(fromRaw, toRaw string) (reportPeriodInput, error) {
	fromValue := strings.TrimSpace(fromRaw)
	toValue := strings.TrimSpace(toRaw)
	if fromValue == "" || toValue == "" {
		return reportPeriodInput{}, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "range report export requires both --report-from and --report-to",
			Details: map[string]any{"required_flags": []string{"report-from", "report-to"}},
		}
	}

	fromUTC, err := normalizeListDateBound(fromValue, false)
	if err != nil {
		return reportPeriodInput{}, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "report-from must be RFC3339 or YYYY-MM-DD",
			Details: map[string]any{"field": "report-from", "value": fromRaw},
		}
	}
	toUTC, err := normalizeListDateBound(toValue, true)
	if err != nil {
		return reportPeriodInput{}, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "report-to must be RFC3339 or YYYY-MM-DD",
			Details: map[string]any{"field": "report-to", "value": toRaw},
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

func buildDataExportReportPresetPeriod(monthRaw, scope string) (reportPeriodInput, error) {
	month := strings.TrimSpace(monthRaw)
	if month == "" {
		return reportPeriodInput{}, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "report-month is required for preset report scopes",
			Details: map[string]any{"field": "report-month"},
		}
	}

	normalizedMonth, err := domain.NormalizeMonthKey(month)
	if err != nil {
		return reportPeriodInput{}, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "report-month must use YYYY-MM",
			Details: map[string]any{"field": "report-month", "value": monthRaw},
		}
	}

	return reportPeriodInput{
		Scope:    scope,
		MonthKey: normalizedMonth,
	}, nil
}

func invalidArgsError(command string, args []string) error {
	return &reportCLIError{
		Code:    "INVALID_ARGUMENT",
		Message: fmt.Sprintf("%s does not accept positional arguments", command),
		Details: map[string]any{"args": args},
	}
}

func restoreDatabase(ctx context.Context, opts *RootOptions, backupPath string) error {
	if ctx == nil {
		return fmt.Errorf("restore db: context is required")
	}
	if opts == nil {
		return fmt.Errorf("restore db: root options are required")
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("restore db: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(opts.DBPath), 0o755); err != nil {
		return err
	}

	tempPath := opts.DBPath + ".restore.tmp"
	rollbackPath := opts.DBPath + ".restore.rollback"
	if err := removeFilesIfExist(tempPath, rollbackPath, rollbackPath+"-wal", rollbackPath+"-shm"); err != nil {
		return err
	}
	defer func() {
		_ = removeFilesIfExist(tempPath)
	}()

	if err := copyFile(tempPath, backupPath); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("restore db: %w", err)
	}

	if opts.db != nil {
		if err := opts.db.Close(); err != nil {
			return err
		}
		opts.db = nil
	}

	rollbackAvailable, err := createRollbackSnapshot(opts.DBPath, rollbackPath)
	if err != nil {
		return err
	}

	if err := os.Rename(tempPath, opts.DBPath); err != nil {
		return err
	}
	_ = os.Remove(opts.DBPath + "-wal")
	_ = os.Remove(opts.DBPath + "-shm")

	db, err := openAndValidateSQLite(ctx, opts.DBPath, opts.MigrationsDir)
	if err != nil {
		rollbackErr := rollbackRestoreFromSnapshot(opts.DBPath, rollbackPath, rollbackAvailable)
		if rollbackErr != nil {
			return fmt.Errorf("restore db validation: %v; rollback failed: %w", err, rollbackErr)
		}

		if rollbackAvailable {
			rollbackDB, reopenErr := openAndValidateSQLite(ctx, opts.DBPath, opts.MigrationsDir)
			if reopenErr != nil {
				return fmt.Errorf("restore db validation: %v; rollback reopen failed: %w", err, reopenErr)
			}
			opts.db = rollbackDB
		}
		if cleanupErr := removeFilesIfExist(rollbackPath, rollbackPath+"-wal", rollbackPath+"-shm"); cleanupErr != nil {
			return fmt.Errorf("restore db validation: %v; rollback cleanup failed: %w", err, cleanupErr)
		}

		return fmt.Errorf("restore db validation: %w", err)
	}
	opts.db = db

	if err := removeFilesIfExist(rollbackPath, rollbackPath+"-wal", rollbackPath+"-shm"); err != nil {
		return err
	}

	return nil
}

func openAndValidateSQLite(ctx context.Context, dbPath, migrationsDir string) (*sql.DB, error) {
	db, err := sqlitestore.OpenAndMigrate(ctx, dbPath, migrationsDir)
	if err != nil {
		return nil, err
	}

	if err := validateSQLiteIntegrity(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func validateSQLiteIntegrity(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("sqlite integrity check: db is nil")
	}

	row, err := db.QueryContext(ctx, "PRAGMA integrity_check;")
	if err != nil {
		return fmt.Errorf("sqlite integrity check query: %w", err)
	}
	defer row.Close()

	var hasRows bool
	for row.Next() {
		var result string
		if err := row.Scan(&result); err != nil {
			return fmt.Errorf("sqlite integrity check scan: %w", err)
		}
		hasRows = true
		if !strings.EqualFold(strings.TrimSpace(result), "ok") {
			return fmt.Errorf("sqlite integrity check failed: %s", result)
		}
	}
	if err := row.Err(); err != nil {
		return fmt.Errorf("sqlite integrity check rows: %w", err)
	}
	if !hasRows {
		return fmt.Errorf("sqlite integrity check failed: no results")
	}

	return nil
}

func createRollbackSnapshot(dbPath, rollbackPath string) (bool, error) {
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	if err := copyFile(rollbackPath, dbPath); err != nil {
		return false, err
	}
	if err := copyFileIfExists(rollbackPath+"-wal", dbPath+"-wal"); err != nil {
		return false, err
	}
	if err := copyFileIfExists(rollbackPath+"-shm", dbPath+"-shm"); err != nil {
		return false, err
	}

	return true, nil
}

func rollbackRestoreFromSnapshot(dbPath, rollbackPath string, rollbackAvailable bool) error {
	if err := removeFilesIfExist(dbPath, dbPath+"-wal", dbPath+"-shm"); err != nil {
		return err
	}
	if !rollbackAvailable {
		return nil
	}

	if err := copyFile(dbPath, rollbackPath); err != nil {
		return err
	}
	if err := copyFileIfExists(dbPath+"-wal", rollbackPath+"-wal"); err != nil {
		return err
	}
	if err := copyFileIfExists(dbPath+"-shm", rollbackPath+"-shm"); err != nil {
		return err
	}

	return nil
}

func removeFilesIfExist(paths ...string) error {
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}

		err := os.Remove(path)
		if err == nil || os.IsNotExist(err) {
			continue
		}
		return err
	}
	return nil
}

func copyFileIfExists(destinationPath, sourcePath string) error {
	if _, err := os.Stat(sourcePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return copyFile(destinationPath, sourcePath)
}

func copyFile(destinationPath, sourcePath string) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(destinationPath)
	if err != nil {
		return err
	}

	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		_ = destinationFile.Close()
		return err
	}
	if err := destinationFile.Close(); err != nil {
		return err
	}

	return nil
}
