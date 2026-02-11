package cli

import (
	"fmt"
	"strings"

	"budgetto/internal/cli/output"
	"budgetto/internal/domain"
	"budgetto/internal/fx"
	"budgetto/internal/service"
	sqlitestore "budgetto/internal/store/sqlite"
	"github.com/spf13/cobra"
)

const (
	balanceScopeLifetime = "lifetime"
	balanceScopeRange    = "range"
	balanceScopeBoth     = "both"
)

type balanceShowFlags struct {
	scope         string
	fromRaw       string
	toRaw         string
	categoryIDRaw string
	labelIDRaw    []string
	labelMode     string
	convertTo     string
}

type balanceCurrencyNet struct {
	CurrencyCode string `json:"currency_code"`
	NetMinor     int64  `json:"net_minor"`
}

type balanceView struct {
	ByCurrency []balanceCurrencyNet `json:"by_currency"`
}

type balanceRangeView struct {
	FromUTC    string               `json:"from_utc,omitempty"`
	ToUTC      string               `json:"to_utc,omitempty"`
	ByCurrency []balanceCurrencyNet `json:"by_currency"`
}

type balanceData struct {
	Scope             string                `json:"scope"`
	Lifetime          *balanceView          `json:"lifetime,omitempty"`
	Range             *balanceRangeView     `json:"range,omitempty"`
	LifetimeConverted *balanceConvertedView `json:"lifetime_converted,omitempty"`
	RangeConverted    *balanceConvertedView `json:"range_converted,omitempty"`
}

type balanceRequest struct {
	Scope         string
	FromUTC       string
	ToUTC         string
	CategoryID    *int64
	LabelIDs      []int64
	LabelMode     string
	ConvertTo     string
	IncludeRange  bool
	IncludeAll    bool
	IncludeGlobal bool
}

type balanceConvertedView struct {
	TargetCurrency   string `json:"target_currency"`
	NetMinor         int64  `json:"net_minor"`
	UsedEstimateRate bool   `json:"used_estimate_rate"`
}

func NewBalanceCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "balance",
		Short: "Show balance views",
	}

	cmd.AddCommand(newBalanceShowCmd(opts))
	return cmd
}

func newBalanceShowCmd(opts *RootOptions) *cobra.Command {
	flags := &balanceShowFlags{}

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show net balance (lifetime, range, or both)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printReportError(cmd, reportOutputFormat(opts), &reportCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "balance show does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			req, err := buildBalanceRequest(flags)
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			svc, err := newBalanceService(opts)
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			result, err := svc.Compute(cmd.Context(), service.BalanceRequest{
				IncludeLifetime: req.IncludeGlobal,
				IncludeRange:    req.IncludeRange,
				RangeFromUTC:    req.FromUTC,
				RangeToUTC:      req.ToUTC,
				CategoryID:      req.CategoryID,
				LabelIDs:        req.LabelIDs,
				LabelMode:       req.LabelMode,
				ConvertTo:       req.ConvertTo,
			})
			if err != nil {
				return printReportError(cmd, reportOutputFormat(opts), err)
			}

			payload := balanceData{Scope: req.Scope}
			if result.Lifetime != nil {
				payload.Lifetime = &balanceView{ByCurrency: toBalanceCurrencyRows(result.Lifetime.ByCurrency)}
			}
			if result.Range != nil {
				payload.Range = &balanceRangeView{
					FromUTC:    req.FromUTC,
					ToUTC:      req.ToUTC,
					ByCurrency: toBalanceCurrencyRows(result.Range.ByCurrency),
				}
			}
			if result.LifetimeConverted != nil {
				payload.LifetimeConverted = &balanceConvertedView{
					TargetCurrency:   result.LifetimeConverted.TargetCurrency,
					NetMinor:         result.LifetimeConverted.NetMinor,
					UsedEstimateRate: result.LifetimeConverted.UsedEstimateRate,
				}
			}
			if result.RangeConverted != nil {
				payload.RangeConverted = &balanceConvertedView{
					TargetCurrency:   result.RangeConverted.TargetCurrency,
					NetMinor:         result.RangeConverted.NetMinor,
					UsedEstimateRate: result.RangeConverted.UsedEstimateRate,
				}
			}

			warnings := []domain.Warning{}
			if (result.LifetimeConverted != nil && result.LifetimeConverted.UsedEstimateRate) ||
				(result.RangeConverted != nil && result.RangeConverted.UsedEstimateRate) {
				warnings = append(warnings, domain.Warning{
					Code:    domain.WarningCodeFXEstimateUsed,
					Message: domain.FXEstimateWarningMessage,
					Details: map[string]any{
						"target_currency": req.ConvertTo,
					},
				})
			}

			env := output.NewSuccessEnvelope(payload, toOutputWarnings(warnings))
			return output.Print(cmd.OutOrStdout(), reportOutputFormat(opts), env)
		},
	}

	cmd.Flags().StringVar(&flags.scope, "scope", balanceScopeBoth, "Balance scope: lifetime|range|both")
	cmd.Flags().StringVar(&flags.fromRaw, "from", "", "Filter start date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.toRaw, "to", "", "Filter end date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&flags.categoryIDRaw, "category-id", "", "Filter by category ID")
	cmd.Flags().StringArrayVar(&flags.labelIDRaw, "label-id", nil, "Filter by label ID (repeatable)")
	cmd.Flags().StringVar(&flags.labelMode, "label-mode", "any", "Label filter mode: any|all|none")
	cmd.Flags().StringVar(&flags.convertTo, "convert-to", "", "Optional target currency (ISO code) for converted net")

	return cmd
}

func newBalanceService(opts *RootOptions) (*service.BalanceService, error) {
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

	fxRepo := sqlitestore.NewFXRepo(opts.db)
	fxClient := fx.NewFrankfurterClient(nil)
	converter, err := fx.NewConverter(fxClient, fxRepo)
	if err != nil {
		return nil, fmt.Errorf("fx converter init: %w", err)
	}

	balanceSvc, err := service.NewBalanceService(entrySvc, service.WithBalanceFXConverter(converter))
	if err != nil {
		return nil, fmt.Errorf("balance service init: %w", err)
	}

	return balanceSvc, nil
}

func buildBalanceRequest(flags *balanceShowFlags) (balanceRequest, error) {
	if flags == nil {
		return balanceRequest{}, &reportCLIError{
			Code:    "INTERNAL_ERROR",
			Message: "balance flags unavailable",
			Details: map[string]any{},
		}
	}

	scope, err := normalizeBalanceScope(flags.scope)
	if err != nil {
		return balanceRequest{}, err
	}

	fromUTC, err := normalizeListDateBound(flags.fromRaw, false)
	if err != nil {
		return balanceRequest{}, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "from must be RFC3339 or YYYY-MM-DD",
			Details: map[string]any{"field": "from", "value": flags.fromRaw},
		}
	}
	toUTC, err := normalizeListDateBound(flags.toRaw, true)
	if err != nil {
		return balanceRequest{}, &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "to must be RFC3339 or YYYY-MM-DD",
			Details: map[string]any{"field": "to", "value": flags.toRaw},
		}
	}
	if err := domain.ValidateDateRange(fromUTC, toUTC); err != nil {
		return balanceRequest{}, err
	}

	var categoryID *int64
	if strings.TrimSpace(flags.categoryIDRaw) != "" {
		id, err := parsePositiveID(flags.categoryIDRaw, "category-id")
		if err != nil {
			return balanceRequest{}, err
		}
		categoryID = &id
	}

	labelIDs, err := parsePositiveIDList(flags.labelIDRaw, "label-id")
	if err != nil {
		return balanceRequest{}, err
	}

	req := balanceRequest{
		Scope:      scope,
		FromUTC:    fromUTC,
		ToUTC:      toUTC,
		CategoryID: categoryID,
		LabelIDs:   labelIDs,
		LabelMode:  flags.labelMode,
		ConvertTo:  flags.convertTo,
		IncludeAll: scope == balanceScopeBoth,
	}

	switch scope {
	case balanceScopeLifetime:
		req.IncludeGlobal = true
	case balanceScopeRange:
		req.IncludeRange = true
	case balanceScopeBoth:
		req.IncludeGlobal = true
		req.IncludeRange = true
	}

	return req, nil
}

func normalizeBalanceScope(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return balanceScopeBoth, nil
	}

	switch normalized {
	case balanceScopeLifetime, balanceScopeRange, balanceScopeBoth:
		return normalized, nil
	default:
		return "", &reportCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "scope must be one of: lifetime|range|both",
			Details: map[string]any{"field": "scope", "value": raw},
		}
	}
}

func toBalanceCurrencyRows(rows []domain.CurrencyNet) []balanceCurrencyNet {
	if len(rows) == 0 {
		return []balanceCurrencyNet{}
	}

	out := make([]balanceCurrencyNet, 0, len(rows))
	for _, row := range rows {
		out = append(out, balanceCurrencyNet{
			CurrencyCode: row.CurrencyCode,
			NetMinor:     row.NetMinor,
		})
	}
	return out
}
