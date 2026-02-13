---
summary: Canonical product and technical specification for boring-budget. This is the single source for behavior and engineering rules.
read_when:
  - Planning feature work or refactors.
  - Changing domain behavior, reporting logic, storage semantics, or CLI contracts.
---

# boring-budget Unified Specification (v2)

## 1) Goal

Build an agent-friendly Go CLI for personal budgeting with:
- SQLite local-first persistence
- deterministic JSON contracts
- multi-currency support
- strong auditability and non-destructive data behavior

## 2) Locked Decisions

- Stack: Go CLI + SQLite.
- Output modes: every command supports `human` and `json`.
- Transactions can be past, present, or future dated.
- Multi-currency is supported per transaction.
- Overspend behavior is allow + warn (never block by default).
- Caps apply to expenses only.
- Cap history appears in relevant reports.
- Bimonthly/quarterly are date-range presets (not separate storage models).
- Deletes are non-destructive for categories/labels/cards.
- Money is stored in minor units only (`amount_minor`) with `currency_code`.
- Converted totals/net are optional and explicit.
- Future-dated expense entries are checked immediately for cap warnings.
- Time is stored in UTC; human rendering may use configured display timezone.
- Data lifecycle uses soft deletes + audit trail.
- SQLite WAL mode is enabled.
- Expense payment method tracking is required:
  - default to `cash` when not specified
  - optional card linkage for expense entries
- Credit card liability is tracked per card and currency.
- Credit card overpayment is allowed and tracked as balance in favor of user.

## 3) Product Scope

### 3.1 Active scope

1. Transaction CRUD with type, amount, currency, and date.
2. Category CRUD.
3. Label CRUD and multi-label assignment.
4. Combined filters (labels + categories + dates).
5. Monthly expense caps with non-blocking overspend warnings.
6. Cap change history visible in reports.
7. Reports split into earnings and spending, with grouping.
8. Balance views for lifetime and date range.
9. Orphan support and orphan-overuse warnings.
10. Expense payment method tracking (`cash` or card).
11. Card registry and lifecycle management.
12. Credit liability tracking and payment events.
13. Card due-day storage and due-date queries.

### 3.2 Post-MVP candidates

- Payment due reminders/notifications.
- Converted multi-currency reporting enhancements for card liabilities.
- Forecasting and trend insights.
- Optional strict mode to reject over-cap entries.

## 4) Domain Rules and Invariants

### 4.1 Transactions

Required fields:
- `type` (`income` or `expense`)
- amount
- currency
- transaction date

Optional fields:
- category
- note
- labels (0..n)
- payment instrument details for expenses

Rules:
- Missing category is treated as `Orphan`.
- A transaction can have multiple labels.
- Transaction updates and deletes are supported.
- Payment method rules:
  - `income`: payment method is not required and does not affect card debt.
  - `expense`: payment method is required logically; default is `cash` if omitted.
  - `expense` + card: card must exist and be active.

### 4.2 Categories and labels

- Categories are dynamic and user-defined.
- Deleting a category does not delete transactions; affected entries become orphaned.
- Labels support create/list/rename/delete.
- Label names are unique case-insensitively.
- Deleting a label does not delete transactions; only links are removed.

### 4.3 Caps and overspend

- Monthly caps are expense-only.
- On add/update of an expense, evaluate that month cap immediately.
- If over cap:
  - write succeeds
  - warning is returned
- Cap updates are allowed anytime and are appended to cap history.

### 4.4 Orphan warning policy

Orphan entries are always allowed.

Emit warning when either threshold is exceeded:
- orphan count > 5 in the period
- orphan spending > 5% of monthly cap or 5% of month spending-so-far

### 4.5 Cards and payment instruments

Card attributes:
- `id` (internal primary key)
- `nickname` (required, unique, case-insensitive)
- `description` (optional, free text)
- `last4` (required, 4 digits)
- `brand` (required, normalized string such as `VISA`, `MASTERCARD`, `DINERS`, `AMEX`, `ELO`, `DISCOVER`, `OTHER`)
- `card_type` (required: `credit` or `debit`)
- `due_day` (required for `credit`, nullable for `debit`)

Rules:
- Cards are soft-deletable; deleting a card does not delete transactions.
- Card updates are allowed for nickname/description/brand/last4/type/due_day, respecting invariants.
- If card type changes, invariant checks apply (for example, `credit` requires `due_day`).

### 4.6 Credit liability and card payments

- Liability is tracked per `(card_id, currency_code)`.
- On expense with `card_type=credit`, create a liability `charge` event for the expense amount.
- On expense with `card_type=debit` or `cash`, no liability event is created.
- Card payment is a dedicated liability event (`payment`), not an income/expense entry.
- Card payment effects:
  - decreases outstanding debt for the specified card+currency bucket
  - if it exceeds debt, resulting bucket balance becomes in favor of user
- Liability balance states:
  - `owes`: balance > 0
  - `settled`: balance = 0
  - `in_favor`: balance < 0
- Payments do not affect income/spending totals and do not affect cap calculations.

### 4.7 Due date rules

- `due_day` is stored as day-of-month (`1..28`) to avoid invalid month-end edge cases.
- Due-date query returns computed next due date based on:
  - card `due_day`
  - current date in display timezone
- Reminders are out of current scope.

## 5) Reporting, Balance, and Queries

Reports must always separate:
- earnings
- spending

Supported report scopes:
- custom range
- monthly
- bimonthly preset
- quarterly preset

Grouping options:
- day
- week
- month

Filters are combinable:
- dates
- categories
- labels
- payment method/card selectors

Label filter modes:
- `ANY`
- `ALL`
- `NONE`

Balance views:
- lifetime
- date range
- both

If currencies are mixed and no conversion is requested, return per-currency values.

### 5.1 Payment-method reporting requirements

Provide card/cash spending reports that include:
- spending grouped by payment instrument (`cash` and each card)
- total spending across all payment instruments
- subtotal by `credit`, `debit`, and `cash`
- outstanding credit liability per card per currency
- in-favor balances when overpayment occurred

Card/payment-method reporting must support time-scoped queries:
- custom range (`--from`, `--to`)
- monthly preset
- bimonthly preset
- quarterly preset

### 5.2 Payment-method querying requirements

Users can query spending by:
- `cash`
- `card_id`
- `card_nickname` (exact match)
- `card_lookup` text (case-insensitive match over nickname, description, and last4)

If lookup is ambiguous, return deterministic conflict/error with candidate cards.

## 6) FX Conversion Rules

- Provider: Frankfurter API (`api.frankfurter.app`) backed by ECB reference data.
- Conversion is optional (`--convert-to <CURRENCY>`).
- Default reporting remains grouped by currency.
- Past/current transactions use historical rate at transaction date.
- Future-dated transactions use latest available rate and must be marked as estimate.
- Persist FX rate snapshots used in conversion for reproducibility.

## 7) Technical Architecture

Package layout:
- `cmd/boring-budget` (entrypoint)
- `internal/cli` (commands/flags/rendering)
- `internal/service` (use cases)
- `internal/domain` (business rules)
- `internal/store/sqlite` (repos + SQL integration)
- `internal/store/sqlite/queries` (SQLC query definitions)
- `internal/store/sqlite/sqlc` (generated SQLC code)
- `internal/reporting` (aggregation/grouping)
- `internal/fx` (provider + conversion)
- `internal/config` (settings/onboarding)
- `migrations` (Goose migrations)
- `docs/contracts` (agent-facing contract examples)

Rules:
- Keep business logic out of CLI handlers.
- Use Goose for migrations.
- Use SQLC for query execution (no hand-written repository CRUD SQL strings).
- Use transactions for multi-step writes.
- Add/maintain indexes for reporting/filter hot paths.
- Serialize writes when needed for concurrent agent operations.

## 8) Data Model (High Level)

Core entities:
- `transactions`
- `categories`
- `labels`
- `transaction_labels`
- `monthly_caps`
- `monthly_cap_changes`
- `settings`
- `fx_rate_snapshots`
- `audit_events`
- `schema_migrations`

Payment-instrument entities:
- `cards`
  - `id`, `nickname` (unique, ci), `description`, `last4`, `brand`, `card_type`, `due_day`, timestamps, `deleted_at_utc`
- `transaction_payment_methods`
  - `transaction_id`, `method_type` (`cash|card`), `card_id` nullable
- `credit_liability_events`
  - `id`, `card_id`, `currency_code`, `event_type` (`charge|payment|adjustment`), `amount_minor_signed`, `reference_transaction_id` nullable, `note` nullable, timestamps

Migration requirement:
- Existing expense records must default to `cash` when migrating to payment-method-aware schema.

## 9) CLI and Agent Contracts

Every command supports:
- `--output human`
- `--output json`

JSON envelope:
- `ok`
- `data`
- `warnings[]`
- `error { code, message, details }`
- `meta { api_version, timestamp_utc }`

Maintain:
- stable exit-code table
- error-code catalog
- contract examples for key commands and new payment-card surfaces

No interactive prompts in core commands by default.

### 9.1 New command surfaces (required)

Card management:
- `card add`
- `card list`
- `card update`
- `card delete`
- `card due show`

Credit liability management:
- `card debt show`
- `card payment add`

Reporting/querying:
- payment-method report variant (or equivalent flags on existing report commands) with time scope support:
  - custom range (`--from`, `--to`)
  - monthly preset
  - bimonthly preset
  - quarterly preset
- entry/report filters for:
  - `cash`
  - `card_id`
  - `card_nickname`
  - `card_lookup` text

## 10) Onboarding and Portability

First-run setup supports:
- default currency
- display timezone
- optional opening balance
- optional current month cap

Data portability supports:
- import: CSV and JSON (including payment method/card metadata)
- export: CSV and JSON (including payment method/card metadata)
- full backup/restore

## 11) Quality and Reliability

- Unit tests for domain rules.
- Integration tests against temporary SQLite DB.
- Golden tests for JSON contracts.
- Deterministic ordering/schema for agent workflows.
- Additional required coverage for this feature:
  - card CRUD + uniqueness invariants
  - expense default-to-cash behavior
  - credit/debit/cash report aggregation
  - credit liability charge/payment/overpayment semantics
  - due-date calculations from `due_day`
  - card lookup ambiguity handling

## 12) Delivery Phases

1. Foundation: schema + migrations for cards/payment methods/liability events.
2. Core flows: card CRUD, expense payment-method capture, default-to-cash migration behavior.
3. Liability flows: credit charge events, payment events, debt/in-favor balances.
4. Reporting/query: payment-method reports, filters, due-date queries.
5. Contracts and hardening: docs/contracts updates, error/exit alignment, tests.
6. Future enhancements: reminder workflows and advanced debt tooling.

## 13) References

- Frankfurter docs: https://frankfurter.dev/docs/
- Frankfurter source: https://github.com/hakanensari/frankfurter
- ECB rates context: https://www.ecb.europa.eu/stats/policy_and_exchange_rates/euro_reference_exchange_rates/html/index.en.html
