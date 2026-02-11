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
- Reporting and balance tests:
  - `internal/domain/report_test.go`
  - `internal/service/report_service_test.go`
  - `internal/service/balance_service_test.go`
  - `internal/cli/report_test.go`
  - `internal/cli/balance_test.go`
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

### Verified

- `go test ./...` passes.
- CLI runs in both human and JSON modes.
- SQLite DB initialization applies WAL mode and base migrations on startup.
- End-to-end command checks pass for `category` and `label` lifecycle operations in JSON mode.
- End-to-end command checks pass for `entry add|list|delete` lifecycle operations in JSON mode.
- End-to-end command checks pass for `cap set|show|history`.
- `entry add` now emits `CAP_EXCEEDED` warning when month expense total is over cap.
- End-to-end command checks pass for `report range|monthly|bimonthly|quarterly`.
- End-to-end command checks pass for `balance show` (`lifetime|range|both` scopes).

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
