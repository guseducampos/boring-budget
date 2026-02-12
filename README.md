# boring-budget

[![Build Status](https://github.com/guseducampos/boring-budget/actions/workflows/ci.yml/badge.svg)](https://github.com/guseducampos/boring-budget/actions/workflows/ci.yml)
[![Version](https://img.shields.io/github/v/release/guseducampos/boring-budget?display_name=tag)](https://github.com/guseducampos/boring-budget/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

![boring-budget hero](docs/assets/boring-budget-hero.webp)

`boring-budget` is a local-first budgeting CLI built with Go + SQLite.

It is intentionally simple:
- Fast CLI workflows for humans
- Stable JSON contracts for agents
- Zero cloud lock-in by default

If you want your money tracking to be reliable, scriptable, and not full of UI noise, this is for you.

## Why Humans Use It

- Expenses and income, in one place
- Categories and multi-label tagging
- Monthly cap warnings without blocking your flow
- Reports that answer real questions quickly
- Local SQLite database you control (`$HOME/.boring-budget/boring-budget.db`)

## Why Agents Like It

- Deterministic JSON envelopes on core commands
- Stable error codes and exit behavior
- Contract-focused docs and regression tests
- Non-destructive data semantics and auditability

## Core Principles

- Money is stored in minor units only (`amount_minor`) + ISO currency code
- Timestamps are stored in UTC
- Human output can render UTC fields in your timezone
- Deletes are non-destructive where possible
- Core commands support both `--output human` and `--output json`

## Feature Summary

- Category CRUD (`add|list|rename|delete`) with orphaning behavior on delete
- Label CRUD (`add|list|rename|delete`) with link detachment on delete
- Entry CRUD (`add|update|list|delete`) with optional category, labels, and note
- Cap management (`set|show|history`) with cap-change history
- Overspend policy: allow write + emit warning
- Reports (`range|monthly|bimonthly|quarterly`) with group-by (`day|week|month`)
- Balance views (`lifetime|range|both`)
- FX conversion via Frankfurter/ECB with persisted rate snapshots
- Onboarding setup (`setup init|show`)
- Portability (`data export|import|backup|restore`) for JSON/CSV + full DB backup
- Hardened restore path with integrity validation + rollback snapshot strategy

## Installation

### Homebrew (easiest)

```bash
brew install guseducampos/tap/boring-budget
```

### Universal binary

- Download the artifact for your platform from CI/release outputs:
  - `darwin/amd64`, `darwin/arm64`
  - `linux/amd64`, `linux/arm64`
- Move the binary to your `PATH` and make it executable on macOS/Linux:

```bash
chmod +x boring-budget
```

Then verify:

```bash
boring-budget --help
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

Get useful output in under a minute.

### 1) Initialize settings

```bash
boring-budget setup init \
  --default-currency USD \
  --timezone America/New_York \
  --opening-balance 1000.00 \
  --opening-balance-date 2026-02-01 \
  --month-cap 500.00 \
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
  --amount 3500.00 \
  --currency USD \
  --date 2026-02-01 \
  --note "Salary"

boring-budget entry add \
  --type expense \
  --amount 12.50 \
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
boring-budget entry add --type income|expense --amount <decimal> --currency <ISO> --date <RFC3339|YYYY-MM-DD> [--category-id <id>] [--label-id <id>] [--note <text>]
boring-budget entry update <id> [--type ...] [--amount ...] [--currency ...] [--date ...] [--category-id <id>|--clear-category] [--label-id <id>|--clear-labels] [--note <text>|--clear-note]
boring-budget entry list [--type ...] [--category-id ...] [--from ...] [--to ...] [--note-contains <text>] [--label-id ...] [--label-mode any|all|none]
boring-budget entry delete <id>
```

Note: `--note` is an optional description field for entries, and `entry list --note-contains` supports case-insensitive substring matching.

### Caps

```bash
boring-budget cap set --month YYYY-MM --amount <decimal> --currency <ISO>
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
  - tests on `ubuntu-latest`, `macos-latest`
  - cross-platform builds for:
    - `darwin/amd64`, `darwin/arm64`
    - `linux/amd64`, `linux/arm64`
- Build artifacts are uploaded per target with SHA256 checksum files.
- Release workflow: `.github/workflows/release.yml`
  - Triggered on `v*` tags
  - Manual runs are validated to require a `v*` tag ref
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

- Docs router (start here): `docs/README.md`
- Unified specification (canonical): `docs/SPEC.md`
- Agent contracts: `docs/contracts/README.md`
- Docs discovery helper: `./scripts/docs-list.sh`
- Changelog: `CHANGELOG.md`

## License

MIT. See `LICENSE`.
