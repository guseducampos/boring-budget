---
summary: Canonical product and technical specification for boring-budget. This is the single source for behavior and engineering rules.
read_when:
  - Planning feature work or refactors.
  - Changing domain behavior, reporting logic, storage semantics, or CLI contracts.
---

# boring-budget Unified Specification (v1)

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
- Deletes are non-destructive for categories/labels.
- Money is stored in minor units only (`amount_minor`) with `currency_code`.
- Converted totals/net are optional and explicit.
- Future-dated expense entries are checked immediately for cap warnings.
- Time is stored in UTC; human rendering may use configured display timezone.
- Data lifecycle uses soft deletes + audit trail.
- SQLite WAL mode is enabled.

## 3) Product Scope

### 3.1 MVP scope

1. Transaction CRUD with type, amount, currency, and date.
2. Category CRUD.
3. Label CRUD and multi-label assignment.
4. Combined filters (labels + categories + dates).
5. Monthly expense caps with non-blocking overspend warnings.
6. Cap change history visible in reports.
7. Reports split into earnings and spending, with grouping.
8. Balance views for lifetime and date range.
9. Orphan support and orphan-overuse warnings.

### 3.2 Post-MVP candidates

- Converted multi-currency reporting enhancements.
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

Rules:
- Missing category is treated as `Orphan`.
- A transaction can have multiple labels.
- Transaction updates and deletes are supported.

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

## 5) Reporting and Balance

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

Label filter modes:
- `ANY`
- `ALL`
- `NONE`

Balance views:
- lifetime
- date range
- both

If currencies are mixed and no conversion is requested, return per-currency values.

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
- transactions
- categories
- labels
- transaction_labels
- monthly_caps
- monthly_cap_changes
- settings
- fx_rate_snapshots
- audit_events
- schema_migrations

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
- contract examples for key commands (`entry add`, `cap set`, `report monthly` and related surfaces)

No interactive prompts in core commands by default.

## 10) Onboarding and Portability

First-run setup supports:
- default currency
- display timezone
- optional opening balance
- optional current month cap

Data portability supports:
- import: CSV and JSON
- export: CSV and JSON
- full backup/restore

## 11) Quality and Reliability

- Unit tests for domain rules.
- Integration tests against temporary SQLite DB.
- Golden tests for JSON contracts.
- Deterministic ordering/schema for agent workflows.

## 12) Delivery Phases

1. Foundation: scaffolding, config, migrations, WAL, JSON envelope.
2. Core CRUD: transactions/categories/labels + non-destructive deletes.
3. Caps/alerts: monthly caps, warnings, cap history.
4. Reporting: scopes, grouping, combined filters, orphan warnings.
5. FX: provider integration, snapshots, converted totals.
6. Agent hardening: contracts, exit/error catalogs, import/export, backup/restore.

## 13) References

- Frankfurter docs: https://frankfurter.dev/docs/
- Frankfurter source: https://github.com/hakanensari/frankfurter
- ECB rates context: https://www.ecb.europa.eu/stats/policy_and_exchange_rates/euro_reference_exchange_rates/html/index.en.html
