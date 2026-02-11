# boring-budget Technical Blueprint (v1)

## 1. Goal

Build an agent-friendly Go CLI for personal budgeting with SQLite storage, multi-currency support, category/label filtering, cap alerts, and strong auditability.

## 2. Locked Decisions

- Stack: Go CLI + SQLite.
- Output modes: `human` and `json` for every command.
- Transaction dates can be past or future.
- Multi-currency supported per transaction.
- Overspend behavior: always allow save, always warn when over cap.
- Caps apply to expenses only.
- Cap history must be shown in reports for queried periods.
- Bimonthly/quarterly are report presets over date ranges (no separate storage model).
- Delete semantics are non-destructive:
  - deleting category -> existing transactions become orphan
  - deleting label -> remove links only, keep transactions
- Money storage uses integer minor units only: `amount_minor`, `currency_code`.
- Converted net is supported when requested.
- Future-dated expense entries are checked immediately for overspend warnings.
- Orphan warnings:
  - count threshold: more than 5 uncategorized entries
  - spend threshold: more than 5% of monthly cap or 5% of month spend-so-far
- Time storage in UTC; responses can be rendered in user-configured timezone.
- Data lifecycle: soft deletes + audit trail.
- SQLite WAL mode enabled.
- Multi-step writes use transactional boundaries; serialized write queue can be added for concurrent agent calls.

## 3. FX Conversion Strategy

### Provider choice

- Primary provider: Frankfurter public API (`api.frankfurter.app`) backed by ECB reference data.
- Rationale: free/open API, latest/historical/range endpoints, self-host option if needed.

### Conversion rules

- Report conversion is optional (`--convert-to <CURRENCY>`).
- Default report mode remains grouped by currency.
- For past/current transactions, convert using transaction-date historical rate.
- For future-dated transactions, use latest available rate and mark as estimate in report metadata.
- Store the rate snapshot used for conversion events to keep reports reproducible.

### Caveat

- ECB reference rates are informational and typically updated daily around 16:00 CET on working days.

## 4. Architecture

- `cmd/boring-budget`: CLI entrypoint.
- `internal/cli`: command handlers, flags, output formatting.
- `internal/service`: use cases and orchestration.
- `internal/domain`: core entities and business rules.
- `internal/store/sqlite`: repositories and SQL layer.
- `internal/reporting`: report builders and grouping logic.
- `internal/fx`: provider client + conversion rules.
- `internal/config`: onboarding and persisted user settings.
- `migrations`: schema evolution.
- `docs/contracts`: command and JSON contract specs for agents.

## 5. Data Model (high level)

- `transactions`
  - `id`, `type`, `amount_minor`, `currency_code`, `transaction_date_utc`
  - `category_id` nullable, `note`, `created_at_utc`, `updated_at_utc`
  - `deleted_at_utc` nullable (soft delete)
- `categories`
  - `id`, `name`, timestamps, `deleted_at_utc`
- `labels`
  - `id`, `name`, timestamps, `deleted_at_utc`
- `transaction_labels`
  - `transaction_id`, `label_id`, timestamps, `deleted_at_utc`
- `monthly_caps`
  - `id`, `month_key` (`YYYY-MM`), `amount_minor`, `currency_code`, timestamps
- `monthly_cap_changes`
  - `id`, `month_key`, `old_amount_minor`, `new_amount_minor`, `currency_code`, `changed_at_utc`
- `settings`
  - default currency, display timezone, orphan thresholds, onboarding state
- `fx_rate_snapshots`
  - `id`, `provider`, `base_currency`, `quote_currency`, `rate`, `rate_date`, `fetched_at_utc`
- `audit_events`
  - action, entity type/id, actor/source, payload diff, timestamp
- `schema_migrations`

## 6. Query and Filter Semantics

- Filters can be combined in one query:
  - date range (`from`, `to`)
  - category (single/multiple)
  - labels (single/multiple)
- Label mode:
  - `ANY`: match any provided label
  - `ALL`: must include all provided labels
  - `NONE`: exclude provided labels

## 7. Budget and Overspend Rules

- Cap is monthly and expense-only.
- Cap checks run on entry add/update, including future dates.
- If an expense pushes month spend above cap:
  - operation succeeds
  - warning includes `cap_amount`, `new_spend_total`, `overspend_amount`.
- Cap updates append to `monthly_cap_changes`.
- Reports over a month/date range include cap history events in that range.

## 8. Reporting Requirements

- Reports must always separate `earnings` and `spending`.
- Supported periods:
  - custom range
  - monthly
  - bimonthly preset
  - quarterly preset
- Date grouping:
  - day, week, month
- Include:
  - orphan bucket
  - cap status/overspend
  - optional converted totals
  - cap change history for included months

## 9. Balance Views

- `lifetime` net
- `range` net
- both selectable by user
- if currencies are mixed and no conversion requested, return per-currency nets

## 10. First-Run Onboarding

- Initialize database and WAL mode.
- Ask/store:
  - default currency (USD/EUR default options)
  - display timezone
  - optional opening balance (amount + currency + date)
  - optional current month cap

## 11. Import / Export

- Import:
  - CSV and JSON transaction import
  - idempotency option (dedupe key or hash)
- Export:
  - CSV and JSON for transactions/reports
  - full backup/restore command for database snapshots

## 12. Agent Contract Requirements

- Every command supports:
  - `--output json`
  - `--output human`
- JSON envelope:
  - `ok`
  - `data`
  - `warnings[]`
  - `error { code, message, details }`
  - `meta { api_version, timestamp_utc }`
- Stable exit codes and documented error catalog.
- Required contract examples:
  - `entry add`
  - `cap set`
  - `report monthly`

## 13. Quality and Reliability

- Use DB transactions for all multi-step writes.
- Add integration tests with temp SQLite DB.
- Add golden tests for JSON outputs.
- Keep deterministic ordering in list/report results for agent use.

## 14. Delivery Phases

1. Foundation
- Project scaffolding, migrations, config, WAL, JSON envelope.

2. Core CRUD
- transactions, categories, labels, non-destructive deletes, soft-delete behavior.

3. Caps and Alerts
- monthly caps, overspend warnings, cap history and report inclusion.

4. Reporting
- earnings/spending split, date grouping, presets, combined filters, orphan warnings.

5. FX and Converted Net
- Frankfurter integration, rate snapshots, converted report/balance output.

6. Agent Hardening
- contract docs, exit-code catalog, examples, import/export, backup/restore.

## 15. Open Notes

- Converted totals rely on reference rates and should be marked as indicative.
- If future scale requires guaranteed uptime or stricter SLA, use provider abstraction and allow self-hosted FX backend.

## 16. References

- Frankfurter API docs and examples: https://frankfurter.dev/docs/
- Frankfurter project (open source): https://github.com/hakanensari/frankfurter
- ECB exchange-rate publication note (daily update context): https://www.ecb.europa.eu/stats/policy_and_exchange_rates/euro_reference_exchange_rates/html/index.en.html
