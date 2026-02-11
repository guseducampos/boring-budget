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

func NewLabelCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Manage labels",
	}

	cmd.AddCommand(
		newLabelAddCmd(opts),
		newLabelListCmd(opts),
		newLabelRenameCmd(opts),
		newLabelDeleteCmd(opts),
	)

	return cmd
}

func AttachLabelCommands(root *cobra.Command, opts *RootOptions) {
	if root == nil {
		return
	}
	root.AddCommand(NewLabelCmd(opts))
}

func newLabelAddCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Create a label",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return printCommandEnvelope(cmd, outputFormat(opts), output.NewErrorEnvelope(
					"INVALID_ARGUMENT",
					"label name is required",
					map[string]any{"field": "name"},
					nil,
				))
			}

			svc, err := newLabelService(opts)
			if err != nil {
				return printCommandEnvelope(cmd, outputFormat(opts), envelopeFromLabelErr(err))
			}

			label, err := svc.Add(cmd.Context(), strings.Join(args, " "))
			if err != nil {
				return printCommandEnvelope(cmd, outputFormat(opts), envelopeFromLabelErr(err))
			}

			env := output.NewSuccessEnvelope(map[string]any{"label": label}, nil)
			return printCommandEnvelope(cmd, outputFormat(opts), env)
		},
	}
}

func newLabelListCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List active labels",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printCommandEnvelope(cmd, outputFormat(opts), output.NewErrorEnvelope(
					"INVALID_ARGUMENT",
					"list does not accept positional arguments",
					map[string]any{"args": args},
					nil,
				))
			}

			svc, err := newLabelService(opts)
			if err != nil {
				return printCommandEnvelope(cmd, outputFormat(opts), envelopeFromLabelErr(err))
			}

			labels, err := svc.List(cmd.Context())
			if err != nil {
				return printCommandEnvelope(cmd, outputFormat(opts), envelopeFromLabelErr(err))
			}

			env := output.NewSuccessEnvelope(map[string]any{
				"labels": labels,
				"count":  len(labels),
			}, nil)
			return printCommandEnvelope(cmd, outputFormat(opts), env)
		},
	}
}

func newLabelRenameCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <id> <new-name>",
		Short: "Rename a label",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return printCommandEnvelope(cmd, outputFormat(opts), output.NewErrorEnvelope(
					"INVALID_ARGUMENT",
					"rename requires <id> and <new-name>",
					map[string]any{"required_args": []string{"id", "new-name"}},
					nil,
				))
			}

			svc, err := newLabelService(opts)
			if err != nil {
				return printCommandEnvelope(cmd, outputFormat(opts), envelopeFromLabelErr(err))
			}

			id, err := parseLabelID(args[0])
			if err != nil {
				return printCommandEnvelope(cmd, outputFormat(opts), envelopeFromLabelErr(err))
			}

			label, err := svc.Rename(cmd.Context(), id, strings.Join(args[1:], " "))
			if err != nil {
				return printCommandEnvelope(cmd, outputFormat(opts), envelopeFromLabelErr(err))
			}

			env := output.NewSuccessEnvelope(map[string]any{"label": label}, nil)
			return printCommandEnvelope(cmd, outputFormat(opts), env)
		},
	}
}

func newLabelDeleteCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft-delete a label and its label links",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return printCommandEnvelope(cmd, outputFormat(opts), output.NewErrorEnvelope(
					"INVALID_ARGUMENT",
					"delete requires exactly one argument: <id>",
					map[string]any{"required_args": []string{"id"}},
					nil,
				))
			}

			svc, err := newLabelService(opts)
			if err != nil {
				return printCommandEnvelope(cmd, outputFormat(opts), envelopeFromLabelErr(err))
			}

			id, err := parseLabelID(args[0])
			if err != nil {
				return printCommandEnvelope(cmd, outputFormat(opts), envelopeFromLabelErr(err))
			}

			result, err := svc.Delete(cmd.Context(), id)
			if err != nil {
				return printCommandEnvelope(cmd, outputFormat(opts), envelopeFromLabelErr(err))
			}

			env := output.NewSuccessEnvelope(map[string]any{"deleted": result}, nil)
			return printCommandEnvelope(cmd, outputFormat(opts), env)
		},
	}
}

func parseLabelID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: id must be a positive integer", domain.ErrInvalidLabelID)
	}

	if err := domain.ValidateLabelID(id); err != nil {
		return 0, err
	}

	return id, nil
}

func newLabelService(opts *RootOptions) (*service.LabelService, error) {
	if opts == nil {
		return nil, fmt.Errorf("%w: cli options unavailable", domain.ErrStorage)
	}

	repo, err := sqlitestore.NewLabelRepo(opts.db)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrStorage, err)
	}

	svc, err := service.NewLabelService(repo)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrStorage, err)
	}

	return svc, nil
}

func printCommandEnvelope(cmd *cobra.Command, format string, env output.Envelope) error {
	if cmd == nil {
		return fmt.Errorf("nil command")
	}
	return output.Print(cmd.OutOrStdout(), format, env)
}

func outputFormat(opts *RootOptions) string {
	if opts == nil {
		return output.FormatHuman
	}
	return opts.Output
}

func envelopeFromLabelErr(err error) output.Envelope {
	switch {
	case errors.Is(err, domain.ErrInvalidLabelName):
		return output.NewErrorEnvelope(
			"INVALID_ARGUMENT",
			"label name is required",
			map[string]any{"field": "name"},
			nil,
		)
	case errors.Is(err, domain.ErrInvalidLabelID):
		return output.NewErrorEnvelope(
			"INVALID_ARGUMENT",
			"label id must be a positive integer",
			map[string]any{"field": "id"},
			nil,
		)
	case errors.Is(err, domain.ErrLabelNotFound):
		return output.NewErrorEnvelope(
			"NOT_FOUND",
			"label not found",
			map[string]any{},
			nil,
		)
	case errors.Is(err, domain.ErrLabelNameConflict):
		return output.NewErrorEnvelope(
			"CONFLICT",
			"label name already exists",
			map[string]any{"field": "name"},
			nil,
		)
	case errors.Is(err, domain.ErrStorage):
		return output.NewErrorEnvelope(
			"DB_ERROR",
			"database operation failed",
			map[string]any{"reason": err.Error()},
			nil,
		)
	default:
		return output.NewErrorEnvelope(
			"INTERNAL_ERROR",
			"unexpected internal failure",
			map[string]any{"reason": err.Error()},
			nil,
		)
	}
}
