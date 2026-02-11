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

## Current Status

Implemented milestones:
1. Go module and Cobra CLI scaffold
2. SQLite bootstrap with WAL + foreign keys
3. Migration runner and initial schema
4. Initial agent contract docs (JSON envelope examples, exit codes, error catalog)
5. Category CRUD (`add|list|rename|delete`) with non-destructive delete semantics
6. Label CRUD (`add|list|rename|delete`) with non-destructive link detachment on delete
7. Entry CRUD (`add|list|delete`) with optional category/labels and combined list filters
