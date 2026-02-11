# AGENTS.md

This file defines implementation rules for agents and contributors working on `boring-budget`.

## 1) Mission

Build an agent-friendly Go CLI for budgeting with:
- SQLite persistence
- deterministic JSON contracts for LLM usage
- strong auditability and non-destructive data behavior

Primary references:
- `docs/PRODUCT_PLAN.md`
- `docs/TECHNICAL_BLUEPRINT.md`

If there is conflict, `TECHNICAL_BLUEPRINT.md` wins for engineering decisions.

## 2) Tech Stack (Locked)

- Language: Go
- App type: CLI
- Database: SQLite
- Storage mode: local-first
- Migrations: Goose (`github.com/pressly/goose/v3`)
- Query layer: SQLC (`github.com/sqlc-dev/sqlc`)

## 3) Architecture Rules

Use this package layout:
- `cmd/boring-budget` (entrypoint)
- `internal/cli` (commands, flags, rendering)
- `internal/service` (use-cases)
- `internal/domain` (business rules/entities)
- `internal/store/sqlite` (queries/repos)
  - `internal/store/sqlite/queries` (SQL source for SQLC)
  - `internal/store/sqlite/sqlc` (generated SQLC code)
- `internal/reporting` (summaries/grouping)
- `internal/fx` (exchange-rate provider integration)
- `internal/config` (onboarding/user settings)
- `migrations` (SQL migrations)
- `docs/contracts` (agent-facing command contracts)

Do not place business logic directly in CLI handlers.

## 4) Data and Domain Invariants (Mandatory)

- Money is stored only as:
  - `amount_minor` (integer minor units)
  - `currency_code` (ISO currency code)
- No float/double storage for money.
- Time is stored in UTC.
- Output timestamps may be rendered in user-configured timezone.
- Transaction date can be past/present/future.
- Categories are optional for transactions:
  - missing category => `Orphan`
- Labels are many-to-many:
  - one transaction can have multiple labels

## 5) Budget and Overspend Rules

- Caps are monthly and apply to expenses only.
- Overspend behavior is non-blocking:
  - always persist the expense
  - return warning when cap exceeded
- Overspend checks also apply to future-dated expenses immediately.
- Cap changes are allowed any time.
- Every cap change must be recorded in cap history.
- Reports over a month/range must show relevant cap changes.

## 6) Reporting Rules

All reports must separate:
- earnings
- spending

Supported scopes:
- custom range
- monthly
- bimonthly preset (range-derived)
- quarterly preset (range-derived)

Grouping options:
- day
- week
- month

Filters must be combinable:
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

## 7) Multi-Currency and FX Rules

- Default report behavior: grouped totals by currency.
- Converted totals/net are supported when requested.
- FX source: Frankfurter API (`api.frankfurter.app`) backed by ECB data.
- Past/current transactions: use historical rate for transaction date.
- Future-dated transactions: use latest available rate and mark as estimate.
- Persist rate snapshots used for conversion to keep reports reproducible.

## 8) Delete and Lifecycle Rules

Use soft deletes for core entities where defined.

Non-destructive semantics:
- deleting a category must not delete transactions
  - affected transactions become orphaned
- deleting a label must not delete transactions
  - only label links are removed

Maintain auditability:
- record key create/update/delete actions in audit events.

## 9) Database and Concurrency Rules

- Enable SQLite WAL mode.
- Use Goose for all schema migrations (no custom/raw migration runner logic).
- Use SQLC for repository query execution (no hand-written CRUD SQL strings in repos).
- Use DB transactions for all multi-step writes.
- Serialize writes when needed for concurrent agent operations.
- Add indexes for reporting/filter hot paths (date/type/category/label joins).

## 10) CLI/Agent Contract Rules

Every command must support:
- `--output human`
- `--output json`

JSON response envelope must include:
- `ok`
- `data`
- `warnings[]`
- `error { code, message, details }`
- `meta { api_version, timestamp_utc }`

Provide and maintain:
- stable exit-code table
- error-code catalog
- example JSON contracts for:
  - `entry add`
  - `cap set`
  - `report monthly`

No interactive prompts in core commands by default.

## 11) Onboarding and Data Portability

First run should support setup of:
- default currency
- display timezone
- optional opening balance
- optional current month cap

Data portability requirements:
- import: CSV and JSON
- export: CSV and JSON
- full backup/restore command

## 12) Orphan Warning Policy

Orphan entries are always allowed.

Emit warning when either threshold is exceeded:
- orphan count > 5 (in period)
- orphan spending > 5% of:
  - monthly cap, or
  - month spending so far

## 13) Testing and Quality Gates

Minimum expectations:
- unit tests for domain rules
- integration tests against temp SQLite DB
- golden tests for JSON output contracts

Behavior must be deterministic for agent calls (stable ordering and schema).

## 14) Implementation Order

1. Foundation: scaffolding, config, migrations, WAL, JSON envelope.
2. Core CRUD: transactions/categories/labels + non-destructive delete behavior.
3. Caps/alerts: monthly caps, overspend warnings, cap history.
4. Reports: scopes, grouping, combined filters, orphan warnings.
5. FX: provider integration, rate snapshots, converted totals.
6. Agent hardening: docs/contracts, exit/error catalogs, import/export, backup/restore.

## 15) Scope Control

Do not add unrelated features without updating:
- `docs/PRODUCT_PLAN.md`
- `docs/TECHNICAL_BLUEPRINT.md`
- this `AGENTS.md` (if rules change)

## 16) Git Commit Policy

- Commits must be atomic:
  - each commit should contain one logical change
  - avoid mixing unrelated refactors/features/fixes in the same commit
  - commit messages should clearly describe that single unit of change

## 17) Changelog Policy

- Keep `CHANGELOG.md` updated for all significant work.
- Add entries under `Unreleased` for:
  - new features
  - behavior changes
  - migration/schema changes
  - notable fixes
- Update changelog as part of the same delivery cycle before marking work complete.
