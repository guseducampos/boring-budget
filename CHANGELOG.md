# Changelog

All notable changes to this project are documented in this file.

The format follows a lightweight Keep a Changelog style.

## [Unreleased]

### Added

- Product and architecture documentation:
  - `docs/PRODUCT_PLAN.md`
  - `docs/TECHNICAL_BLUEPRINT.md`
  - `AGENTS.md`
- Project bootstrap docs:
  - `README.md`
  - `docs/contracts/README.md`
  - `docs/contracts/entry-add.json`
  - `docs/contracts/cap-set.json`
  - `docs/contracts/report-monthly.json`
  - `docs/contracts/errors.md`
  - `docs/contracts/exit-codes.md`
- Go project scaffold:
  - `go.mod` / `go.sum`
  - `cmd/budgetto/main.go`
  - `internal/cli/root.go`
  - `internal/cli/output/envelope.go`
  - `internal/cli/output/print.go`
  - `internal/config/path.go`
- SQLite persistence foundation:
  - `internal/store/sqlite/db.go`
  - `internal/store/sqlite/migrate.go`
  - `migrations/0001_initial.sql`
- Initial tests:
  - `internal/cli/output/envelope_test.go`
  - `internal/store/sqlite/migrate_test.go`
- Category CRUD implementation:
  - `internal/domain/category.go`
  - `internal/service/category_service.go`
  - `internal/store/sqlite/category_repo.go`
  - `internal/cli/category.go`
- Label CRUD implementation:
  - `internal/domain/label.go`
  - `internal/service/label_service.go`
  - `internal/store/sqlite/label_repo.go`
  - `internal/cli/label.go`
- SQLC query layer:
  - `sqlc.yaml`
  - `internal/store/sqlite/queries/category.sql`
  - `internal/store/sqlite/queries/label.sql`
  - `internal/store/sqlite/sqlc/*` (generated code)
- Entry CRUD implementation:
  - `internal/domain/entry.go`
  - `internal/service/entry_service.go`
  - `internal/store/sqlite/entry_repo.go`
  - `internal/cli/entry.go`
- Cap management implementation:
  - `internal/domain/cap.go`
  - `internal/service/cap_service.go`
  - `internal/store/sqlite/cap_repo.go`
  - `internal/cli/cap.go`
- Reporting and balance implementation:
  - `internal/domain/report.go`
  - `internal/service/report_service.go`
  - `internal/service/balance_service.go`
  - `internal/cli/report.go`
  - `internal/cli/balance.go`
- FX conversion implementation:
  - `internal/domain/fx.go`
  - `internal/fx/converter.go`
  - `internal/fx/frankfurter.go`
  - `internal/store/sqlite/fx_repo.go`
  - `internal/store/sqlite/queries/fx.sql`
  - `internal/store/sqlite/sqlc/fx.sql.go`
- Setup/onboarding implementation:
  - `internal/domain/settings.go`
  - `internal/service/setup_service.go`
  - `internal/store/sqlite/settings_repo.go`
  - `internal/store/sqlite/queries/settings.sql`
  - `internal/store/sqlite/sqlc/settings.sql.go`
  - `internal/cli/setup.go`
- Data portability implementation:
  - `internal/service/portability_service.go`
  - `internal/cli/data.go`
- Entry update + exit code enforcement:
  - `internal/cli/output/exit_code.go`
  - `cmd/budgetto/main.go`
  - `internal/cli/entry.go`
  - `internal/service/entry_service.go`
  - `internal/store/sqlite/entry_repo.go`
  - `internal/store/sqlite/queries/entry.sql`
- Timezone-aware human rendering and tests:
  - `internal/cli/output/display_timezone.go`
  - `internal/cli/output/print_test.go`
- Reporting architecture extraction:
  - `internal/reporting/aggregate.go`
- Auditability enforcement:
  - `migrations/0003_audit_triggers.sql`
- Reporting and balance tests:
  - `internal/domain/report_test.go`
  - `internal/service/report_service_test.go`
  - `internal/service/balance_service_test.go`
  - `internal/cli/report_test.go`
  - `internal/cli/balance_test.go`
- FX tests:
  - `internal/fx/converter_test.go`
  - `internal/store/sqlite/fx_repo_test.go`
- Settings tests:
  - `internal/store/sqlite/settings_repo_test.go`
- Audit trigger tests:
  - `internal/store/sqlite/audit_triggers_test.go`
- Entry update and exit-code tests:
  - `internal/cli/entry_test.go`
  - `internal/service/entry_service_test.go`
  - `internal/store/sqlite/entry_repo_test.go`
  - `internal/cli/output/exit_code_test.go`
- Data CLI regression tests:
  - `internal/cli/data_test.go`
- Golden JSON contract coverage for key agent commands:
  - `internal/cli/json_contracts_golden_test.go`
  - `internal/cli/testdata/json_contracts/*`
- Golden JSON contract coverage now includes `data export --resource report` with deterministic normalization for volatile timestamps and temp file paths.
- Portability atomic-import tests:
  - `internal/service/portability_service_test.go`
- Data portability report export contract:
  - `docs/contracts/data-export-report.json`
- SQLC entry query layer:
  - `internal/store/sqlite/queries/entry.sql`
  - `internal/store/sqlite/sqlc/entry.sql.go`
- SQLC cap query layer:
  - `internal/store/sqlite/queries/cap.sql`
  - `internal/store/sqlite/sqlc/cap.sql.go`
- Migration:
  - `migrations/0002_transaction_labels_transaction_active_index.sql`
- Phase 2 tests:
  - `internal/service/category_service_test.go`
  - `internal/store/sqlite/category_repo_test.go`
  - `internal/domain/label_test.go`
  - `internal/service/label_service_test.go`
  - `internal/store/sqlite/label_repo_test.go`
  - `internal/cli/label_test.go`
- Repository hygiene:
  - `.gitignore`

### Changed

- `AGENTS.md` now enforces atomic commit policy.
- `README.md` updated with quick-start instructions and contract docs link.
- Migration engine switched from custom raw SQL runner to Goose (`github.com/pressly/goose/v3`).
- Migration file `migrations/0001_initial.sql` converted to Goose `Up/Down` format.
- Root CLI now registers `category` and `label` command groups.
- Category/label repositories migrated from embedded raw CRUD SQL strings to SQLC-generated queries.
- Root CLI now registers the `entry` command group.
- Root CLI now registers the `cap` command group.
- Root CLI now registers `report` and `balance` command groups.
- Report and balance commands now delegate aggregation logic to service-layer use cases.
- Report contracts updated to match implemented JSON payload shape (`categories`, cap status/history, warnings envelope).
- SQLC config now includes `entry`, `cap`, and `fx` query sources (`sqlc.yaml`).
- `report` and `balance` now support `--convert-to` with persisted FX rate snapshots.
- SQLC config now includes `settings` query sources (`sqlc.yaml`).
- Root CLI now registers `setup` and `data` command groups.
- Report aggregation logic moved under `internal/reporting` to match architecture layout rules.
- DB-level triggers now write key create/update/delete events into `audit_events` for categories/labels/entries/caps/settings.
- Added strict process exit-code mapping from JSON envelope error codes (`INVALID_ARGUMENT`→2, `NOT_FOUND`→3, etc.).
- Added `entry update` command with partial updates and clear/set semantics for category, labels, and note.
- Human output now localizes `*_utc` fields using CLI display timezone while JSON output remains UTC.
- Root command now defaults display timezone from persisted settings when `--timezone` is not explicitly provided.
- Report service now reads orphan warning thresholds from settings when available.
- Report command wiring now injects settings reader consistently, including FX-enabled report mode.
- `data export` now supports `--resource report` with report-specific flags and file exports in `json|csv`, while preserving entry export behavior.
- Portability import transaction binding now uses explicit typed interfaces (`EntryRepositoryTxBinder` / `EntryCapLookupTxBinder`) instead of reflection-based `BindTx` lookup.

### Verified

- `go test ./...` passes.
- CLI runs in both human and JSON modes.
- SQLite DB initialization applies WAL mode and base migrations on startup.
- End-to-end command checks pass for `category` and `label` lifecycle operations in JSON mode.
- End-to-end command checks pass for `entry add|list|delete` lifecycle operations in JSON mode.
- End-to-end command checks pass for `cap set|show|history`.
- `entry add` now emits `CAP_EXCEEDED` warning when month expense total is over cap.
- `data` command group now has JSON regression coverage for export/import/idempotent flows and backup/restore.
- `data import` now runs as an atomic batch transaction (all-or-nothing) and rolls back on mid-batch failures.
- SQLite entry and cap repositories now support transaction binding (`BindTx`) for composed write flows.
- End-to-end command checks pass for `report range|monthly|bimonthly|quarterly`.
- End-to-end command checks pass for `balance show` (`lifetime|range|both` scopes).
- `go test ./...` covers FX conversion logic and FX snapshot repository behavior.
- End-to-end command checks pass for `setup init|show` and `data export|import|backup|restore`.
- Migration and integration checks pass for audit trigger writes to `audit_events`.
- Binary-level validation confirms non-zero mapped exit codes on command failures.
- `go test ./...` covers timezone localization behavior and settings-driven orphan threshold evaluation.
- `go test ./...` covers `data export --resource report` for JSON/CSV file generation and warning propagation.

## Progress Notes

- Milestone completed: **Phase 1 Foundation**
  - CLI skeleton with deterministic output envelope
  - SQLite setup (WAL + foreign keys + migration runner)
  - Initial schema aligned with product/blueprint decisions
  - Agent contract docs and baseline tests

- Milestone completed: **Phase 2 Core CRUD (Category + Label)**
  - Category and label command groups integrated in CLI
  - Non-destructive delete rules implemented:
    - deleting category orphans active transactions
    - deleting label soft-deletes label links
  - Service and repository test coverage added for both domains

- Milestone completed: **Phase 3 Core CRUD (Entry)**
  - Entry command group integrated in CLI (`add|list|delete`)
  - Optional category/label assignment on add
  - Combined list filters: type, category, date range, labels (`any|all|none`)
  - Non-destructive delete with link detachment
  - Service and repository test coverage added for entry domain

- Milestone completed: **Phase 4 Caps and Alerts**
  - Monthly cap commands integrated in CLI (`set|show|history`)
  - Cap change history stored and queryable by month
  - `entry add` preserves allow+warn behavior and returns `CAP_EXCEEDED` warning when applicable
  - Service and repository test coverage added for caps and warning behavior

- Milestone completed: **Phase 4 Reporting and Balance**
  - Report commands integrated in CLI (`range|monthly|bimonthly|quarterly`)
  - Grouping supported by `day|week|month` with combinable category/date/label filters
  - Report output includes earnings/spending/net split, cap status, and cap change history
  - Orphan threshold warnings added (`ORPHAN_COUNT_THRESHOLD_EXCEEDED`, `ORPHAN_SPENDING_THRESHOLD_EXCEEDED`)
  - Balance command integrated with `lifetime|range|both` scopes and per-currency net output

- Milestone completed: **Phase 5 FX and Converted Net**
  - Frankfurter provider integration added for optional report/balance conversion (`--convert-to`)
  - Historical rates used for past/current transactions; latest rate used for future-dated transactions
  - FX rate snapshots persisted in SQLite (`fx_rate_snapshots`) for reproducible conversions
  - `FX_ESTIMATE_USED` warning emitted when future-dated conversion uses latest available rate

- Milestone completed: **Phase 6 Agent Hardening (Core)**
  - Setup onboarding flow added (`setup init|show`) with default currency/timezone + optional opening balance/month cap
  - Data portability commands added (`data export|import|backup|restore`) for CSV/JSON + full SQLite backup lifecycle
  - Contract examples extended for setup/data command responses
  - Architecture compliance improved with dedicated `internal/reporting` package
  - Auditability strengthened with DB-triggered `audit_events` writes for key mutations
