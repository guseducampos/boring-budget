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
- Repository hygiene:
  - `.gitignore`

### Changed

- `AGENTS.md` now enforces atomic commit policy.
- `README.md` updated with quick-start instructions and contract docs link.

### Verified

- `go test ./...` passes.
- CLI runs in both human and JSON modes.
- SQLite DB initialization applies WAL mode and base migrations on startup.

## Progress Notes

- Milestone completed: **Phase 1 Foundation**
  - CLI skeleton with deterministic output envelope
  - SQLite setup (WAL + foreign keys + migration runner)
  - Initial schema aligned with product/blueprint decisions
  - Agent contract docs and baseline tests
