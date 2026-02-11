# Budgetto

Agent-friendly personal budgeting CLI built with Go and SQLite.

## Overview

Budgetto is a local-first CLI for tracking:
- expenses and earnings
- categories and labels
- monthly caps and overspend alerts
- reporting by date range, month, bimonthly, and quarterly presets

The project is designed for both humans and LLM agents, with deterministic command behavior and structured JSON output contracts.

## Project Docs

- Product plan: `docs/PRODUCT_PLAN.md`
- Technical blueprint: `docs/TECHNICAL_BLUEPRINT.md`
- Agent contracts: `docs/contracts/README.md`
- Contributor/agent rules: `AGENTS.md`

## Key Principles

- Money is stored in integer minor units (`amount_minor`) with `currency_code`.
- Timestamps are stored in UTC and rendered in user-configured timezone.
- Overspend checks warn but do not block writes.
- Deletes are non-destructive where possible.
- Core commands support both human and JSON output modes.

## Quick Start

1. Run in human mode:
```bash
go run ./cmd/budgetto --output human
```
2. Run in JSON mode:
```bash
go run ./cmd/budgetto --output json
```
3. Optional explicit paths:
```bash
go run ./cmd/budgetto --db-path "$HOME/.budgetto/budgetto.db" --migrations-dir "./migrations"
```

## Current Commands

- Categories:
```bash
go run ./cmd/budgetto category add "Food"
go run ./cmd/budgetto category list
go run ./cmd/budgetto category rename 1 "Groceries"
go run ./cmd/budgetto category delete 1
```
- Labels:
```bash
go run ./cmd/budgetto label add "Recurring"
go run ./cmd/budgetto label list
go run ./cmd/budgetto label rename 1 "Fixed"
go run ./cmd/budgetto label delete 1
```
- Entries:
```bash
go run ./cmd/budgetto entry add --type expense --amount-minor 1200 --currency USD --date 2026-02-11 --category-id 1 --label-id 2 --note "Lunch"
go run ./cmd/budgetto entry list --type expense --from 2026-02-01 --to 2026-02-28 --label-id 2 --label-mode any
go run ./cmd/budgetto entry delete 1
```
- Caps:
```bash
go run ./cmd/budgetto cap set --month 2026-02 --amount-minor 50000 --currency USD
go run ./cmd/budgetto cap show --month 2026-02
go run ./cmd/budgetto cap history --month 2026-02
```
- Reports:
```bash
go run ./cmd/budgetto report range --from 2026-02-01 --to 2026-02-28 --group-by day
go run ./cmd/budgetto report monthly --month 2026-02 --group-by month
go run ./cmd/budgetto report bimonthly --month 2026-02 --group-by week
go run ./cmd/budgetto report quarterly --month 2026-01 --group-by month
go run ./cmd/budgetto report monthly --month 2026-02 --group-by month --convert-to EUR
```
- Balance:
```bash
go run ./cmd/budgetto balance show --scope both --from 2026-02-01 --to 2026-02-28
go run ./cmd/budgetto balance show --scope lifetime
go run ./cmd/budgetto balance show --scope range --from 2026-02-01 --to 2026-02-28 --convert-to USD
```
- Setup:
```bash
go run ./cmd/budgetto setup init --default-currency USD --timezone UTC --opening-balance-minor 100000 --month-cap-minor 50000
go run ./cmd/budgetto setup show
```
- Data Portability:
```bash
go run ./cmd/budgetto data export --format json --file ./backup/entries.json --from 2026-01-01 --to 2026-12-31
go run ./cmd/budgetto data import --format csv --file ./seed/entries.csv --idempotent
go run ./cmd/budgetto data backup --file ./backup/budgetto.db
go run ./cmd/budgetto data restore --file ./backup/budgetto.db
```

## Current Status

Implemented milestones:
1. Go module and Cobra CLI scaffold
2. SQLite bootstrap with WAL + foreign keys
3. Migration runner and initial schema
4. Initial agent contract docs (JSON envelope examples, exit codes, error catalog)
5. Category CRUD (`add|list|rename|delete`) with non-destructive delete semantics
6. Label CRUD (`add|list|rename|delete`) with non-destructive link detachment on delete
7. Entry CRUD (`add|list|delete`) with optional category/labels and combined list filters
8. Monthly cap management (`set|show|history`) and `CAP_EXCEEDED` warning on `entry add`
9. Reporting command group (`range|monthly|bimonthly|quarterly`) with grouping/filter options
10. Balance command group (`show`) with `lifetime|range|both` views
11. FX conversion (`--convert-to`) for report and balance with persisted snapshot rates
12. Setup command group (`init|show`) for onboarding settings + optional opening balance/current month cap
13. Data command group (`export|import|backup|restore`) for CSV/JSON portability and full SQLite snapshot lifecycle
