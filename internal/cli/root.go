package cli

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"budgetto/internal/cli/output"
	"budgetto/internal/config"
	sqlitestore "budgetto/internal/store/sqlite"
	"github.com/spf13/cobra"
)

type RootOptions struct {
	Output        string
	Timezone      string
	DBPath        string
	MigrationsDir string

	db *sql.DB
}

func NewRootCmd() *cobra.Command {
	defaultDBPath, err := config.DefaultDBPath()
	if err != nil {
		defaultDBPath = config.DefaultDBFile
	}

	opts := &RootOptions{
		Output:        output.FormatHuman,
		Timezone:      "UTC",
		DBPath:        defaultDBPath,
		MigrationsDir: sqlitestore.DefaultMigrationsDir,
	}

	cmd := &cobra.Command{
		Use:           "budgetto",
		Short:         "Budgetto is a local-first budgeting CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if !output.IsValidFormat(opts.Output) {
				return fmt.Errorf("invalid --output value %q: supported values are %s|%s", opts.Output, output.FormatHuman, output.FormatJSON)
			}

			if _, err := time.LoadLocation(opts.Timezone); err != nil {
				return fmt.Errorf("invalid --timezone value %q: %w", opts.Timezone, err)
			}

			opts.Output = strings.ToLower(strings.TrimSpace(opts.Output))
			db, err := sqlitestore.OpenAndMigrate(cmd.Context(), opts.DBPath, opts.MigrationsDir)
			if err != nil {
				return fmt.Errorf("initialize sqlite: %w", err)
			}

			opts.db = db
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			envelope := output.NewSuccessEnvelope(
				map[string]any{
					"command":        "root",
					"message":        "budgetto CLI foundation ready",
					"timezone":       opts.Timezone,
					"db_path":        opts.DBPath,
					"migrations_dir": opts.MigrationsDir,
				},
				nil,
			)

			return output.Print(cmd.OutOrStdout(), opts.Output, envelope)
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			if opts.db != nil {
				if err := opts.db.Close(); err != nil {
					return fmt.Errorf("close sqlite db: %w", err)
				}
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&opts.Output, "output", output.FormatHuman, "Output format: human|json")
	cmd.PersistentFlags().StringVar(&opts.Timezone, "timezone", "UTC", "Display timezone (IANA, e.g. America/New_York)")
	cmd.PersistentFlags().StringVar(&opts.DBPath, "db-path", opts.DBPath, "SQLite database path")
	cmd.PersistentFlags().StringVar(&opts.MigrationsDir, "migrations-dir", opts.MigrationsDir, "Migrations directory path")

	cmd.AddCommand(
		NewCategoryCmd(opts),
		NewLabelCmd(opts),
		NewEntryCmd(opts),
		NewCapCmd(opts),
		NewReportCmd(opts),
		NewBalanceCmd(opts),
		NewSetupCmd(opts),
		NewDataCmd(opts),
	)

	return cmd
}
