# boring-budget

`boring-budget` is a local-first budgeting CLI built with Go + SQLite.
your boring budget tracking for you and your agent.

It is designed for two audiences:
- Humans who want a practical command-line budgeting tool.
- Agents/LLMs that need deterministic JSON contracts and stable automation behavior.

## Why boring-budget

- Track both expenses and earnings.
- Organize transactions with categories and multiple labels.
- Enforce monthly spending caps with non-blocking overspend warnings.
- Generate reports across range/monthly/bimonthly/quarterly scopes.
- Support multi-currency reporting and optional FX-converted summaries.
- Keep data local by default (`$HOME/.boring-budget/boring-budget.db`).

## Core Principles

- Money is stored in minor units only (`amount_minor`) plus `currency_code`.
- Timestamps are stored in UTC.
- Human output can render `*_utc` values in configured timezone.
- Deletes are non-destructive where possible.
- Core commands support both `--output human` and `--output json`.
- JSON shape and ordering are contract-driven and regression tested.

## Feature Summary

- Category CRUD (`add|list|rename|delete`) with orphaning behavior on delete.
- Label CRUD (`add|list|rename|delete`) with link detachment on delete.
- Entry CRUD (`add|update|list|delete`) with optional category, labels, and note.
- Cap management (`set|show|history`) with cap-change history.
- Overspend policy: allow write + emit warning.
- Reports (`range|monthly|bimonthly|quarterly`) with group-by (`day|week|month`).
- Balance views (`lifetime|range|both`).
- FX conversion via Frankfurter/ECB with persisted rate snapshots.
- Onboarding setup (`setup init|show`).
- Portability (`data export|import|backup|restore`) for JSON/CSV + full DB backup.
- Hardened restore path with integrity validation + rollback snapshot strategy.

## Installation

### Universal binary (recommended)

- Download the artifact for your platform from CI/release outputs:
  - `darwin/amd64`, `darwin/arm64`
  - `linux/amd64`, `linux/arm64`
  - `windows/amd64`
- Move the binary to your `PATH` and make it executable on macOS/Linux:

```bash
chmod +x boring-budget
```

Then verify:

```bash
boring-budget --help
```

### Homebrew

Once releases are published, install with:

```bash
brew tap guseducampos/tap
brew install boring-budget
```

Or one-liner:

```bash
brew install guseducampos/tap/boring-budget
```

### Build from source (development)

Prerequisite:
- Go `1.24+`

```bash
go install ./cmd/boring-budget
```

Or run directly:

```bash
go run ./cmd/boring-budget --help
```

## Quick Start

### 1) Initialize settings

```bash
boring-budget setup init \
  --default-currency USD \
  --timezone America/New_York \
  --opening-balance-minor 100000 \
  --opening-balance-date 2026-02-01 \
  --month-cap-minor 50000 \
  --month-cap-month 2026-02
```

### 2) Create categories and labels

```bash
boring-budget category add "Food"
boring-budget label add "Recurring"
```

### 3) Add entries

```bash
boring-budget entry add \
  --type income \
  --amount-minor 350000 \
  --currency USD \
  --date 2026-02-01 \
  --note "Salary"

boring-budget entry add \
  --type expense \
  --amount-minor 1250 \
  --currency USD \
  --date 2026-02-11 \
  --category-id 1 \
  --label-id 1 \
  --note "Lunch"
```

### 4) Run reports

```bash
boring-budget report monthly --month 2026-02 --group-by month
boring-budget balance show --scope both --from 2026-02-01 --to 2026-02-28
```

## Command Guide

### Global flags

```bash
--output human|json
--timezone <IANA TZ>
--db-path <sqlite file>
--migrations-dir <path>
```

### Categories

```bash
boring-budget category add <name>
boring-budget category list
boring-budget category rename <id> <new-name>
boring-budget category delete <id>
```

### Labels

```bash
boring-budget label add <name>
boring-budget label list
boring-budget label rename <id> <new-name>
boring-budget label delete <id>
```

### Entries

```bash
boring-budget entry add --type income|expense --amount-minor <int> --currency <ISO> --date <RFC3339|YYYY-MM-DD> [--category-id <id>] [--label-id <id>] [--note <text>]
boring-budget entry update <id> [--type ...] [--amount-minor ...] [--currency ...] [--date ...] [--category-id <id>|--clear-category] [--label-id <id>|--clear-labels] [--note <text>|--clear-note]
boring-budget entry list [--type ...] [--category-id ...] [--from ...] [--to ...] [--note-contains <text>] [--label-id ...] [--label-mode any|all|none]
boring-budget entry delete <id>
```

Note: `--note` is an optional description field for entries, and `entry list --note-contains` supports case-insensitive substring matching.

### Caps

```bash
boring-budget cap set --month YYYY-MM --amount-minor <int> --currency <ISO>
boring-budget cap show --month YYYY-MM
boring-budget cap history --month YYYY-MM
```

### Reports

```bash
boring-budget report range --from <date> --to <date> [--group-by day|week|month] [--category-id <id>] [--label-id <id>] [--label-mode any|all|none] [--convert-to <ISO>]
boring-budget report monthly --month YYYY-MM [...same optional filters...]
boring-budget report bimonthly --month YYYY-MM [...same optional filters...]
boring-budget report quarterly --month YYYY-MM [...same optional filters...]
```

### Balance

```bash
boring-budget balance show --scope lifetime|range|both [--from <date>] [--to <date>] [--category-id <id>] [--label-id <id>] [--label-mode any|all|none] [--convert-to <ISO>]
```

### Setup

```bash
boring-budget setup init --default-currency <ISO> --timezone <IANA>
boring-budget setup show
```

### Data portability

```bash
boring-budget data export --resource entries --format json|csv --file <path> [--from <date>] [--to <date>]
boring-budget data export --resource report --format json|csv --file <path> --report-scope range|monthly|bimonthly|quarterly [scope flags] [filters]
boring-budget data import --format json|csv --file <path> [--idempotent]
boring-budget data backup --file <path>
boring-budget data restore --file <path>
```

Restore details:
- Uses command context for cancellation.
- Validates DB with `PRAGMA integrity_check` before success.
- Rolls back to pre-restore snapshot if validation fails.

## Agent/LLM Integration

`boring-budget` is contract-first for JSON mode.

### JSON envelope

```json
{
  "ok": true,
  "data": {},
  "warnings": [],
  "error": null,
  "meta": {
    "api_version": "v1",
    "timestamp_utc": "<timestamp_utc>"
  }
}
```

Contract docs and examples:
- `docs/contracts/README.md`
- `docs/contracts/errors.md`
- `docs/contracts/exit-codes.md`

## Architecture

Primary package layout:
- `cmd/boring-budget`: CLI entrypoint
- `internal/cli`: Cobra commands, input parsing, rendering
- `internal/service`: use-case orchestration
- `internal/domain`: entities and business invariants
- `internal/store/sqlite`: SQLC-backed repositories
- `internal/reporting`: deterministic aggregation logic
- `internal/fx`: exchange-rate provider + converter
- `internal/config`: local config path defaults
- `migrations`: Goose migrations
- `docs/contracts`: agent-facing contract examples

## Development

### Test

```bash
go test ./...
```

### CI

- GitHub Actions workflow: `.github/workflows/ci.yml`
- On `pull_request` and `push` to `master`, CI runs:
  - tests on `ubuntu-latest`, `macos-latest`, `windows-latest`
  - cross-platform builds for:
    - `darwin/amd64`, `darwin/arm64`
    - `linux/amd64`, `linux/arm64`
    - `windows/amd64`
- Build artifacts are uploaded per target with SHA256 checksum files.
- Release workflow: `.github/workflows/release.yml`
  - Triggered on `v*` tags
  - Publishes GitHub release assets using GoReleaser
  - Updates Homebrew formula in `guseducampos/homebrew-tap` (requires `HOMEBREW_TAP_GITHUB_TOKEN` secret)

### Important implementation notes

- Migrations are managed with Goose.
- Repository query layer uses SQLC.
- JSON contract determinism is guarded by golden tests and docs-sync tests.
- Project-level implementation rules live in `AGENTS.md`.

## Contributing

Issues and PRs are welcome.

Before opening a PR:
- run `go test ./...`
- keep commits atomic
- update `CHANGELOG.md` for significant changes
- update/add contracts under `docs/contracts` when JSON behavior changes

## Documentation

- Product plan: `docs/PRODUCT_PLAN.md`
- Technical blueprint: `docs/TECHNICAL_BLUEPRINT.md`
- Changelog: `CHANGELOG.md`
