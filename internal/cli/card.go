package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"boring-budget/internal/cli/output"
	"boring-budget/internal/domain"
	"boring-budget/internal/service"
	sqlitestore "boring-budget/internal/store/sqlite"
	"github.com/spf13/cobra"
)

type cardAddFlags struct {
	nickname    string
	description string
	last4       string
	brand       string
	cardType    string
	dueDayRaw   string
}

type cardListFlags struct {
	lookup         string
	cardType       string
	includeDeleted bool
}

type cardUpdateFlags struct {
	nickname    string
	description string
	clearDesc   bool
	last4       string
	brand       string
	cardType    string
	dueDayRaw   string
	clearDueDay bool
}

type cardSelectorFlags struct {
	cardIDRaw    string
	cardNickname string
	cardLookup   string
}

type cardDueFlags struct {
	cardSelectorFlags
	asOf string
}

type cardDebtFlags struct {
	cardSelectorFlags
}

type cardPaymentFlags struct {
	cardSelectorFlags
	amount   string
	currency string
	note     string
}

type cardCLIError struct {
	Code    string
	Message string
	Details any
}

func (e *cardCLIError) Error() string {
	if e == nil {
		return "card command error"
	}
	return e.Message
}

func NewCardCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "card",
		Short: "Manage cards and card liabilities",
	}

	dueCmd := &cobra.Command{
		Use:   "due",
		Short: "Card due-date queries",
	}
	dueCmd.AddCommand(newCardDueShowCmd(opts), newCardDueListCmd(opts))

	debtCmd := &cobra.Command{
		Use:   "debt",
		Short: "Card debt queries",
	}
	debtCmd.AddCommand(newCardDebtShowCmd(opts))

	paymentCmd := &cobra.Command{
		Use:   "payment",
		Short: "Card payment operations",
	}
	paymentCmd.AddCommand(newCardPaymentAddCmd(opts))

	cmd.AddCommand(
		newCardAddCmd(opts),
		newCardListCmd(opts),
		newCardUpdateCmd(opts),
		newCardDeleteCmd(opts),
		dueCmd,
		debtCmd,
		paymentCmd,
	)

	return cmd
}

func newCardAddCmd(opts *RootOptions) *cobra.Command {
	flags := &cardAddFlags{}

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a card",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printCardError(cmd, opts.Output, &cardCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "card add does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newCardService(opts)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			if strings.TrimSpace(flags.nickname) == "" {
				return printCardError(cmd, opts.Output, &cardCLIError{Code: "INVALID_ARGUMENT", Message: "nickname is required", Details: map[string]any{"field": "nickname"}})
			}
			if strings.TrimSpace(flags.last4) == "" {
				return printCardError(cmd, opts.Output, &cardCLIError{Code: "INVALID_ARGUMENT", Message: "last4 is required", Details: map[string]any{"field": "last4"}})
			}
			if strings.TrimSpace(flags.brand) == "" {
				return printCardError(cmd, opts.Output, &cardCLIError{Code: "INVALID_ARGUMENT", Message: "brand is required", Details: map[string]any{"field": "brand"}})
			}
			if strings.TrimSpace(flags.cardType) == "" {
				return printCardError(cmd, opts.Output, &cardCLIError{Code: "INVALID_ARGUMENT", Message: "card-type is required", Details: map[string]any{"field": "card-type"}})
			}

			dueDay, err := parseCardDueDay(flags.dueDayRaw)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			card, err := svc.Add(cmd.Context(), domain.CardAddInput{
				Nickname:    flags.nickname,
				Description: flags.description,
				Last4:       flags.last4,
				Brand:       flags.brand,
				CardType:    flags.cardType,
				DueDay:      dueDay,
			})
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{"card": card}, nil))
		},
	}

	cmd.Flags().StringVar(&flags.nickname, "nickname", "", "Unique nickname for this card")
	cmd.Flags().StringVar(&flags.description, "description", "", "Optional description")
	cmd.Flags().StringVar(&flags.last4, "last4", "", "Card last 4 digits")
	cmd.Flags().StringVar(&flags.brand, "brand", "", "Card brand (e.g. VISA, MASTERCARD, DINERS)")
	cmd.Flags().StringVar(&flags.cardType, "card-type", "", "Card type: credit|debit")
	cmd.Flags().StringVar(&flags.dueDayRaw, "due-day", "", "Due day of month (1..28), required for credit cards")

	return cmd
}

func newCardListCmd(opts *RootOptions) *cobra.Command {
	flags := &cardListFlags{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List cards",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printCardError(cmd, opts.Output, &cardCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "card list does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newCardService(opts)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			cards, err := svc.List(cmd.Context(), domain.CardListFilter{
				Lookup:         flags.lookup,
				CardType:       flags.cardType,
				IncludeDeleted: flags.includeDeleted,
			})
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{
				"cards": cards,
				"count": len(cards),
			}, nil))
		},
	}

	cmd.Flags().StringVar(&flags.lookup, "lookup", "", "Optional lookup text over nickname/description/last4")
	cmd.Flags().StringVar(&flags.cardType, "card-type", "", "Optional card type filter: credit|debit")
	cmd.Flags().BoolVar(&flags.includeDeleted, "include-deleted", false, "Include soft-deleted cards")

	return cmd
}

func newCardUpdateCmd(opts *RootOptions) *cobra.Command {
	flags := &cardUpdateFlags{}

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update card metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return printCardError(cmd, opts.Output, &cardCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "card update requires exactly one argument: <id>",
					Details: map[string]any{"required_args": []string{"id"}},
				})
			}

			cardID, err := parsePositiveCardID(args[0], "id")
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			if cmd.Flags().Changed("clear-description") && cmd.Flags().Changed("description") {
				return printCardError(cmd, opts.Output, &cardCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "clear-description cannot be used with description",
					Details: map[string]any{"fields": []string{"clear-description", "description"}},
				})
			}
			if cmd.Flags().Changed("clear-due-day") && cmd.Flags().Changed("due-day") {
				return printCardError(cmd, opts.Output, &cardCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "clear-due-day cannot be used with due-day",
					Details: map[string]any{"fields": []string{"clear-due-day", "due-day"}},
				})
			}

			svc, err := newCardService(opts)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			input := domain.CardUpdateInput{ID: cardID}
			if cmd.Flags().Changed("nickname") {
				value := flags.nickname
				input.Nickname = &value
			}
			if cmd.Flags().Changed("clear-description") {
				input.SetDescription = true
				input.Description = nil
			}
			if cmd.Flags().Changed("description") {
				value := flags.description
				input.SetDescription = true
				input.Description = &value
			}
			if cmd.Flags().Changed("last4") {
				value := flags.last4
				input.Last4 = &value
			}
			if cmd.Flags().Changed("brand") {
				value := flags.brand
				input.Brand = &value
			}
			if cmd.Flags().Changed("card-type") {
				value := flags.cardType
				input.CardType = &value
			}
			if cmd.Flags().Changed("clear-due-day") {
				input.SetDueDay = true
				input.DueDay = nil
			}
			if cmd.Flags().Changed("due-day") {
				dueDay, err := parseCardDueDay(flags.dueDayRaw)
				if err != nil {
					return printCardError(cmd, opts.Output, err)
				}
				input.SetDueDay = true
				input.DueDay = dueDay
			}

			card, err := svc.Update(cmd.Context(), input)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{"card": card}, nil))
		},
	}

	cmd.Flags().StringVar(&flags.nickname, "nickname", "", "New card nickname")
	cmd.Flags().StringVar(&flags.description, "description", "", "New optional description")
	cmd.Flags().BoolVar(&flags.clearDesc, "clear-description", false, "Clear description")
	cmd.Flags().StringVar(&flags.last4, "last4", "", "New card last 4 digits")
	cmd.Flags().StringVar(&flags.brand, "brand", "", "New brand")
	cmd.Flags().StringVar(&flags.cardType, "card-type", "", "New card type: credit|debit")
	cmd.Flags().StringVar(&flags.dueDayRaw, "due-day", "", "New due day (1..28)")
	cmd.Flags().BoolVar(&flags.clearDueDay, "clear-due-day", false, "Clear due day")

	return cmd
}

func newCardDeleteCmd(opts *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft-delete a card",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return printCardError(cmd, opts.Output, &cardCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "card delete requires exactly one argument: <id>",
					Details: map[string]any{"required_args": []string{"id"}},
				})
			}

			cardID, err := parsePositiveCardID(args[0], "id")
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			svc, err := newCardService(opts)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			deleted, err := svc.Delete(cmd.Context(), cardID)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{"card_delete": deleted}, nil))
		},
	}
}

func newCardDueShowCmd(opts *RootOptions) *cobra.Command {
	flags := &cardDueFlags{}

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show next due date for one card",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printCardError(cmd, opts.Output, &cardCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "card due show does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			selector, err := buildCardSelector(flags.cardSelectorFlags)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			svc, err := newCardService(opts)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			card, err := svc.Resolve(cmd.Context(), selector)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			due, err := svc.ShowDue(cmd.Context(), card.ID, flags.asOf, opts.Timezone)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{"due": due}, nil))
		},
	}

	bindCardSelectorFlags(cmd, &flags.cardSelectorFlags)
	cmd.Flags().StringVar(&flags.asOf, "as-of", "", "Reference date (YYYY-MM-DD or RFC3339), default now")

	return cmd
}

func newCardDueListCmd(opts *RootOptions) *cobra.Command {
	flags := &cardDueFlags{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List next due date for active credit cards",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printCardError(cmd, opts.Output, &cardCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "card due list does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newCardService(opts)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			dues, err := svc.ListDues(cmd.Context(), flags.asOf, opts.Timezone)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{
				"dues":  dues,
				"count": len(dues),
			}, nil))
		},
	}

	cmd.Flags().StringVar(&flags.asOf, "as-of", "", "Reference date (YYYY-MM-DD or RFC3339), default now")
	return cmd
}

func newCardDebtShowCmd(opts *RootOptions) *cobra.Command {
	flags := &cardDebtFlags{}

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show debt summary for one card or all cards",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printCardError(cmd, opts.Output, &cardCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "card debt show does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			svc, err := newCardService(opts)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			hasSelector := strings.TrimSpace(flags.cardIDRaw) != "" || strings.TrimSpace(flags.cardNickname) != "" || strings.TrimSpace(flags.cardLookup) != ""
			if hasSelector {
				selector, err := buildCardSelector(flags.cardSelectorFlags)
				if err != nil {
					return printCardError(cmd, opts.Output, err)
				}
				card, err := svc.Resolve(cmd.Context(), selector)
				if err != nil {
					return printCardError(cmd, opts.Output, err)
				}

				debt, err := svc.ShowDebtByCard(cmd.Context(), card.ID)
				if err != nil {
					return printCardError(cmd, opts.Output, err)
				}

				return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{
					"debt": debt,
				}, nil))
			}

			allDebt, err := svc.ShowDebtAll(cmd.Context())
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{
				"debts": allDebt,
				"count": len(allDebt),
			}, nil))
		},
	}

	bindCardSelectorFlags(cmd, &flags.cardSelectorFlags)
	return cmd
}

func newCardPaymentAddCmd(opts *RootOptions) *cobra.Command {
	flags := &cardPaymentFlags{currency: defaultEntryCurrency}

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Register a credit-card payment",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return printCardError(cmd, opts.Output, &cardCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "card payment add does not accept positional arguments",
					Details: map[string]any{"args": args},
				})
			}

			if !cmd.Flags().Changed("amount") {
				return printCardError(cmd, opts.Output, &cardCLIError{
					Code:    "INVALID_ARGUMENT",
					Message: "amount is required",
					Details: map[string]any{"field": "amount"},
				})
			}

			selector, err := buildCardSelector(flags.cardSelectorFlags)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			svc, err := newCardService(opts)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			card, err := svc.Resolve(cmd.Context(), selector)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			amountMinor, err := domain.ParseMajorAmountToMinor(flags.amount, flags.currency)
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			result, err := svc.AddPayment(cmd.Context(), domain.CardPaymentAddInput{
				CardID:            card.ID,
				CurrencyCode:      flags.currency,
				AmountMinorSigned: amountMinor,
				Note:              flags.note,
			})
			if err != nil {
				return printCardError(cmd, opts.Output, err)
			}

			return output.Print(cmd.OutOrStdout(), opts.Output, output.NewSuccessEnvelope(map[string]any{
				"payment": result,
			}, nil))
		},
	}

	bindCardSelectorFlags(cmd, &flags.cardSelectorFlags)
	cmd.Flags().StringVar(&flags.amount, "amount", "", "Payment amount in major units (required)")
	cmd.Flags().StringVar(&flags.currency, "currency", defaultEntryCurrency, "Payment currency")
	cmd.Flags().StringVar(&flags.note, "note", "", "Optional note")

	return cmd
}

func bindCardSelectorFlags(cmd *cobra.Command, flags *cardSelectorFlags) {
	if cmd == nil || flags == nil {
		return
	}

	cmd.Flags().StringVar(&flags.cardIDRaw, "card-id", "", "Card ID selector")
	cmd.Flags().StringVar(&flags.cardNickname, "card-nickname", "", "Card nickname selector (exact)")
	cmd.Flags().StringVar(&flags.cardLookup, "card-lookup", "", "Card lookup selector (nickname/description/last4)")
}

func buildCardSelector(flags cardSelectorFlags) (domain.CardSelector, error) {
	var cardID *int64
	if strings.TrimSpace(flags.cardIDRaw) != "" {
		id, err := parsePositiveCardID(flags.cardIDRaw, "card-id")
		if err != nil {
			return domain.CardSelector{}, err
		}
		cardID = &id
	}

	selector, err := domain.NormalizeCardSelector(domain.CardSelector{
		ID:       cardID,
		Nickname: flags.cardNickname,
		Lookup:   flags.cardLookup,
	})
	if err != nil {
		return domain.CardSelector{}, err
	}
	return selector, nil
}

func parsePositiveCardID(raw, field string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, &cardCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: fmt.Sprintf("%s is required", field),
			Details: map[string]any{"field": field},
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, &cardCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: fmt.Sprintf("%s must be a positive integer", field),
			Details: map[string]any{"field": field, "value": raw},
		}
	}
	return parsed, nil
}

func parseCardDueDay(raw string) (*int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil || value <= 0 {
		return nil, &cardCLIError{
			Code:    "INVALID_ARGUMENT",
			Message: "due-day must be an integer between 1 and 28",
			Details: map[string]any{"field": "due-day", "value": raw},
		}
	}
	out := value
	return &out, nil
}

func newCardService(opts *RootOptions) (*service.CardService, error) {
	if opts == nil || opts.db == nil {
		return nil, &cardCLIError{
			Code:    "DB_ERROR",
			Message: "database operation failed",
			Details: map[string]any{"reason": "database connection unavailable"},
		}
	}

	svc, err := service.NewCardService(sqlitestore.NewCardRepo(opts.db))
	if err != nil {
		return nil, err
	}
	return svc, nil
}

func printCardError(cmd *cobra.Command, format string, err error) error {
	if cmd == nil {
		return fmt.Errorf("nil command")
	}

	if err == nil {
		env := output.NewErrorEnvelope("INTERNAL_ERROR", "unexpected internal failure", map[string]any{}, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	var cliErr *cardCLIError
	if errors.As(err, &cliErr) {
		env := output.NewErrorEnvelope(cliErr.Code, cliErr.Message, cliErr.Details, nil)
		return output.Print(cmd.OutOrStdout(), format, env)
	}

	env := output.NewErrorEnvelope(codeFromCardError(err), messageFromCardError(err), map[string]any{"reason": err.Error()}, nil)
	return output.Print(cmd.OutOrStdout(), format, env)
}

func codeFromCardError(err error) string {
	switch {
	case errors.Is(err, domain.ErrCardNotFound):
		return "NOT_FOUND"
	case errors.Is(err, domain.ErrCardNicknameConflict), errors.Is(err, domain.ErrCardLookupAmbiguous):
		return "CONFLICT"
	case errors.Is(err, domain.ErrInvalidCurrencyCode):
		return "INVALID_CURRENCY_CODE"
	case errors.Is(err, domain.ErrInvalidCardID),
		errors.Is(err, domain.ErrCardNicknameRequired),
		errors.Is(err, domain.ErrCardLast4Invalid),
		errors.Is(err, domain.ErrCardBrandRequired),
		errors.Is(err, domain.ErrInvalidCardBrand),
		errors.Is(err, domain.ErrInvalidCardType),
		errors.Is(err, domain.ErrInvalidCardDueDay),
		errors.Is(err, domain.ErrCardDueDayRequiredForCredit),
		errors.Is(err, domain.ErrCardDueDayOnlyForCredit),
		errors.Is(err, domain.ErrNoCardUpdateFields),
		errors.Is(err, domain.ErrCardLookupRequired),
		errors.Is(err, domain.ErrCardLookupSelectorConflict),
		errors.Is(err, domain.ErrInvalidCardLookupText),
		errors.Is(err, domain.ErrInvalidCardAsOfDate),
		errors.Is(err, domain.ErrCardPaymentRequiresCredit),
		errors.Is(err, domain.ErrInvalidCardPaymentAmount):
		return "INVALID_ARGUMENT"
	default:
		return "DB_ERROR"
	}
}

func messageFromCardError(err error) string {
	switch {
	case errors.Is(err, domain.ErrCardNotFound):
		return "card not found"
	case errors.Is(err, domain.ErrCardNicknameConflict):
		return "card nickname already exists"
	case errors.Is(err, domain.ErrCardLookupAmbiguous):
		return "card lookup matches multiple cards"
	case errors.Is(err, domain.ErrInvalidCurrencyCode):
		return "currency must be a 3-letter ISO code"
	case errors.Is(err, domain.ErrInvalidCardID):
		return "card id must be a positive integer"
	case errors.Is(err, domain.ErrCardNicknameRequired):
		return "nickname is required"
	case errors.Is(err, domain.ErrCardLast4Invalid):
		return "last4 must be exactly 4 digits"
	case errors.Is(err, domain.ErrCardBrandRequired):
		return "brand is required"
	case errors.Is(err, domain.ErrInvalidCardBrand):
		return "brand is invalid"
	case errors.Is(err, domain.ErrInvalidCardType):
		return "card-type must be one of: credit|debit"
	case errors.Is(err, domain.ErrInvalidCardDueDay):
		return "due-day must be between 1 and 28"
	case errors.Is(err, domain.ErrCardDueDayRequiredForCredit):
		return "due-day is required for credit cards"
	case errors.Is(err, domain.ErrCardDueDayOnlyForCredit):
		return "due-day is only allowed for credit cards"
	case errors.Is(err, domain.ErrNoCardUpdateFields):
		return "at least one update field is required"
	case errors.Is(err, domain.ErrCardLookupRequired):
		return "card selector is required"
	case errors.Is(err, domain.ErrCardLookupSelectorConflict):
		return "card-id, card-nickname and card-lookup are mutually exclusive"
	case errors.Is(err, domain.ErrInvalidCardLookupText):
		return "card-lookup cannot be empty"
	case errors.Is(err, domain.ErrInvalidCardAsOfDate):
		return "as-of must be YYYY-MM-DD or RFC3339"
	case errors.Is(err, domain.ErrCardPaymentRequiresCredit):
		return "card payment requires a credit card"
	case errors.Is(err, domain.ErrInvalidCardPaymentAmount):
		return "payment amount must be greater than zero"
	default:
		return "database operation failed"
	}
}
