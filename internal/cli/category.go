package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"budgetto/internal/cli/output"
	"budgetto/internal/domain"
	"budgetto/internal/service"
	sqlitestore "budgetto/internal/store/sqlite"
	"github.com/spf13/cobra"
)

func NewCategoryCmd(opts *RootOptions) *cobra.Command {
	command := &cobra.Command{
		Use:   "category",
		Short: "Manage categories",
	}

	command.AddCommand(
		newCategoryAddCmd(opts),
		newCategoryListCmd(opts),
		newCategoryRenameCmd(opts),
		newCategoryDeleteCmd(opts),
	)

	return command
}

func newCategoryAddCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Create a category",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return printCategoryError(cmd, opts.Output, "INVALID_ARGUMENT", "category name is required", map[string]any{"field": "name"})
			}

			categoryService := service.NewCategoryService(sqlitestore.NewCategoryRepo(opts.db))
			category, err := categoryService.Add(cmd.Context(), strings.Join(args, " "))
			if err != nil {
				return printCategoryServiceError(cmd, opts.Output, err)
			}

			return printCategorySuccess(cmd, opts.Output, map[string]any{"category": category})
		},
	}
}

func newCategoryListCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List active categories",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printCategoryError(cmd, opts.Output, "INVALID_ARGUMENT", "list does not accept positional arguments", map[string]any{"args": args})
			}

			categoryService := service.NewCategoryService(sqlitestore.NewCategoryRepo(opts.db))
			categories, err := categoryService.List(cmd.Context())
			if err != nil {
				return printCategoryServiceError(cmd, opts.Output, err)
			}

			return printCategorySuccess(cmd, opts.Output, map[string]any{
				"categories": categories,
				"count":      len(categories),
			})
		},
	}
}

func newCategoryRenameCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <id> <new-name>",
		Short: "Rename an active category",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return printCategoryError(
					cmd,
					opts.Output,
					"INVALID_ARGUMENT",
					"rename requires <id> and <new-name>",
					map[string]any{"required_args": []string{"id", "new-name"}},
				)
			}

			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return printCategoryError(cmd, opts.Output, "INVALID_ARGUMENT", "id must be a positive integer", map[string]any{"field": "id", "value": args[0]})
			}

			categoryService := service.NewCategoryService(sqlitestore.NewCategoryRepo(opts.db))
			category, err := categoryService.Rename(cmd.Context(), id, strings.Join(args[1:], " "))
			if err != nil {
				return printCategoryServiceError(cmd, opts.Output, err)
			}

			return printCategorySuccess(cmd, opts.Output, map[string]any{"category": category})
		},
	}
}

func newCategoryDeleteCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft-delete a category and orphan active transactions",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return printCategoryError(
					cmd,
					opts.Output,
					"INVALID_ARGUMENT",
					"delete requires exactly one argument: <id>",
					map[string]any{"required_args": []string{"id"}},
				)
			}

			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return printCategoryError(cmd, opts.Output, "INVALID_ARGUMENT", "id must be a positive integer", map[string]any{"field": "id", "value": args[0]})
			}

			categoryService := service.NewCategoryService(sqlitestore.NewCategoryRepo(opts.db))
			result, err := categoryService.Delete(cmd.Context(), id)
			if err != nil {
				return printCategoryServiceError(cmd, opts.Output, err)
			}

			return printCategorySuccess(cmd, opts.Output, map[string]any{"category_delete": result})
		},
	}
}

func printCategorySuccess(cmd *cobra.Command, format string, data any) error {
	envelope := output.NewSuccessEnvelope(data, nil)
	return output.Print(cmd.OutOrStdout(), format, envelope)
}

func printCategoryServiceError(cmd *cobra.Command, format string, err error) error {
	code := "DB_ERROR"
	message := "category operation failed"
	details := map[string]any{"error": err.Error()}

	switch {
	case errors.Is(err, domain.ErrInvalidCategoryID):
		code = "INVALID_ARGUMENT"
		message = "category id must be a positive integer"
	case errors.Is(err, domain.ErrCategoryNameRequired):
		code = "INVALID_ARGUMENT"
		message = "category name is required"
	case errors.Is(err, domain.ErrCategoryNameTooLong):
		code = "INVALID_ARGUMENT"
		message = fmt.Sprintf("category name must be at most %d characters", domain.CategoryNameMaxLength)
	case errors.Is(err, domain.ErrCategoryNotFound):
		code = "NOT_FOUND"
		message = "category not found"
	case errors.Is(err, domain.ErrCategoryNameConflict):
		code = "CONFLICT"
		message = "category name already exists"
	}

	return printCategoryError(cmd, format, code, message, details)
}

func printCategoryError(cmd *cobra.Command, format, code, message string, details any) error {
	envelope := output.NewErrorEnvelope(code, message, details, nil)
	return output.Print(cmd.OutOrStdout(), format, envelope)
}
