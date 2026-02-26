# boring-budget

[![Build Status](https://github.com/guseducampos/boring-budget/actions/workflows/ci.yml/badge.svg)](https://github.com/guseducampos/boring-budget/actions/workflows/ci.yml)
[![Version](https://img.shields.io/github/v/release/guseducampos/boring-budget?display_name=tag)](https://github.com/guseducampos/boring-budget/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

![boring-budget hero](docs/assets/boring-budget-hero.webp)

`boring-budget` is a local-first budgeting CLI built with Go + SQLite.

It is designed for people who want:
- Fast terminal workflows
- Reliable local storage
- Scriptable output for automation and agents

No dashboard noise. No cloud lock-in required.

## What It Is

`boring-budget` is a deterministic money ledger for people and agents who prefer truth over UI abstraction.

It helps you:
- Capture daily money movement with explicit rules
- Track card liabilities and savings without hidden side effects
- Generate reproducible reports across time windows and currencies
- Automate budgeting workflows safely through stable JSON contracts

## What Makes It Different

- Local-first, single SQLite database under your control
- Agent-safe contract model (`ok`, `data`, `warnings`, `error`, `meta`)
- Non-destructive semantics for key entities (soft deletes, orphan-safe behavior)
- Overspend policy that warns instead of blocking writes
- First-class support for cards, debt, savings, bank-account attribution, and fixed schedules
- Deterministic export/import and backup/restore for portability and recovery

## Capability Areas

- Ledger and taxonomy: income/expense entries, categories, labels, and combined filters
- Budget control: monthly caps, cap history, and warning-based overspend feedback
- Liability control: credit/debit card registry, due-day queries, debt summaries, payment events
- Savings and attribution: transfers, direct savings adds, optional bank-account links
- Predictable reporting: range/monthly/bimonthly/quarterly scopes with day/week/month grouping
- Automation-ready output: stable error codes and exit behavior for workflows and agents

## Agent Skill (OpenClaw and Others)

Use the built-in skill pack when running this project with OpenClaw or any compatible coding/automation agent:

- Skill file: `skills/boring-budget-agent/SKILL.md`
- Workflow playbook: `skills/boring-budget-agent/references/workflows.md`
- OpenClaw/OpenAI agent profile: `skills/boring-budget-agent/agents/openai.yaml`

Recommended skill invocation:

```text
$boring-budget-agent
```

This skill enforces deterministic JSON mode, stable error/exit handling, and safe write semantics.

## Installation

### Homebrew

```bash
brew install guseducampos/tap/boring-budget
```

### Build from source

Prerequisite: Go `1.24+`

```bash
go install ./cmd/boring-budget
```

### Verify

```bash
boring-budget --help
```

## First Run

```bash
boring-budget setup init \
  --default-currency USD \
  --timezone America/New_York \
  --opening-balance 1000.00
```

Then inspect state:

```bash
boring-budget setup show
boring-budget report monthly --month 2026-02
```

## Command Reference

- Full command catalog moved to: `docs/COMMANDS.md`
- Quick runtime help: `boring-budget --help`
- Command-level help: `boring-budget <command> --help`

## JSON Contract Surface

For automation flows, use `--output json`.

Core envelope:

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

Contracts and examples:
- `docs/contracts/README.md`
- `docs/contracts/errors.md`
- `docs/contracts/exit-codes.md`
- `docs/contracts/*.json`

## Data Location and Safety

- Default DB path: `$HOME/.boring-budget/boring-budget.db`
- SQLite WAL mode enabled
- UTC storage with timezone-aware rendering
- Soft deletes for key entities
- Restore includes integrity checks and rollback strategy

## Development

```bash
gofmt -w <edited-files>.go
go test ./...
```

Canonical behavior specs and contracts:
- `docs/SPEC.md`
- `docs/contracts/README.md`

## License

[MIT](./LICENSE)
