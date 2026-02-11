package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"budgetto/internal/cli/output"
	"budgetto/internal/domain"
	"budgetto/internal/service"
	sqlitestore "budgetto/internal/store/sqlite"
	"github.com/spf13/cobra"
)

type dataExportFlags struct {
	format string
	file   string
	from   string
	to     string
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
		Short: "Export entries to JSON or CSV",
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

			portabilitySvc, err := newPortabilityService(opts)
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			count, err := portabilitySvc.Export(cmd.Context(), flags.format, flags.file, domain.EntryListFilter{DateFromUTC: fromUTC, DateToUTC: toUTC})
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			env := output.NewSuccessEnvelope(map[string]any{"exported": count, "format": strings.ToLower(flags.format), "file": flags.file}, nil)
			return output.Print(cmd.OutOrStdout(), reportOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.format, "format", "", "Export format: json|csv")
	cmd.Flags().StringVar(&flags.file, "file", "", "Output file path")
	cmd.Flags().StringVar(&flags.from, "from", "", "Optional filter start date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.to, "to", "", "Optional filter end date (RFC3339 or YYYY-MM-DD)")

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

			if err := restoreDatabase(opts, flags.file); err != nil {
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

	portabilitySvc, err := service.NewPortabilityService(entrySvc, opts.db)
	if err != nil {
		return nil, fmt.Errorf("portability service init: %w", err)
	}

	return portabilitySvc, nil
}

func invalidArgsError(command string, args []string) error {
	return &reportCLIError{
		Code:    "INVALID_ARGUMENT",
		Message: fmt.Sprintf("%s does not accept positional arguments", command),
		Details: map[string]any{"args": args},
	}
}

func restoreDatabase(opts *RootOptions, backupPath string) error {
	if opts == nil {
		return fmt.Errorf("restore db: root options are required")
	}

	sourceFile, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	if opts.db != nil {
		if err := opts.db.Close(); err != nil {
			return err
		}
		opts.db = nil
	}

	if err := os.MkdirAll(filepath.Dir(opts.DBPath), 0o755); err != nil {
		return err
	}

	tempPath := opts.DBPath + ".restore.tmp"
	destinationFile, err := os.Create(tempPath)
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

	if err := os.Rename(tempPath, opts.DBPath); err != nil {
		return err
	}

	_ = os.Remove(opts.DBPath + "-wal")
	_ = os.Remove(opts.DBPath + "-shm")

	db, err := sqlitestore.OpenAndMigrate(context.Background(), opts.DBPath, opts.MigrationsDir)
	if err != nil {
		return err
	}
	opts.db = db

	return nil
}
