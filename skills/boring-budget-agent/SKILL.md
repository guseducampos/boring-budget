---
name: boring-budget-agent
description: Operate the boring-budget Go CLI in deterministic, agent-safe mode using stable JSON envelopes, exit/error codes, and non-destructive data semantics. Use when tasks involve setup, category/label/entry/cap workflows, reports and balances, or data import/export/backup/restore for boring-budget.
metadata: {"openclaw":{"emoji":"💸","homepage":"https://github.com/guseducampos/boring-budget","requires":{"bins":["boring-budget"]}},"clawdbot":{"emoji":"💸","homepage":"https://github.com/guseducampos/boring-budget","requires":{"commands":["boring-budget"]}}}
---

# Boring Budget Agent

Use this skill to run `boring-budget` commands with deterministic behavior for AI workflows.

## Runtime environments

1. Repository mode:
   - The `boring-budget` repo is available locally.
   - Use `docs/contracts/*` as source of truth when needed.
2. Binary-only mode:
   - Only the `boring-budget` binary is available.
   - Do not assume repository files exist.
   - Use `boring-budget --help` plus `--output json` responses as runtime contract source.

## Install and bootstrap (required)

1. Check availability:
   - `command -v boring-budget`
2. If missing, install before proceeding:
   - Homebrew: `brew install guseducampos/tap/boring-budget`
   - or download the release binary from GitHub Releases and put it on `PATH`
3. If Homebrew install fails, fall back to release binary installation and continue.
4. Verify installation:
   - `boring-budget --help`
5. Verify JSON mode:
   - run a lightweight command: `boring-budget setup show --output json`

## Required operating mode

1. Prefer `--output json` for all automation flows.
2. Treat `ok`, `warnings[]`, `error`, and `meta` as the canonical response envelope.
3. Persist and validate money in minor units only (`amount_minor`) with ISO currency codes.
4. For command inputs, use decimal amount flags (`--amount`, `--opening-balance`, `--month-cap`) and rely on CLI-side deterministic conversion/validation to canonical `amount_minor`.
5. Legacy minor-unit input flags are removed from command input (`--amount-minor`, `--opening-balance-minor`, `--month-cap-minor`); do not use them.
6. When updating entry amounts, send `--currency` together with `--amount` so conversion is explicit and deterministic.
7. Use UTC-safe dates/timestamps for deterministic runs.
8. Never assume deletes are destructive:
   - deleting a category orphans linked entries
   - deleting a label removes links only
9. Treat overspend warnings as non-blocking writes (`CAP_EXCEEDED` warns, does not fail).

## Standard execution flow

1. Ensure binary is available; install if missing (see install section).
2. Ensure setup exists (or run it once):
   - `boring-budget setup show --output json`
   - if missing, run `setup init` with explicit currency/timezone.
3. Use explicit flags in commands (no interactive assumptions).
4. On write commands, inspect `warnings[]` even when `ok=true`.
5. On failures (`ok=false`), branch from `error.code` and stable exit mapping.

## High-value command patterns

```bash
# Setup
boring-budget setup init --default-currency USD --timezone America/New_York --output json

# Categories and labels
boring-budget category add "Food" --output json
boring-budget label add "Recurring" --output json

# Add income/expense entries
boring-budget entry add --type income --amount 3500.00 --currency USD --date 2026-02-01 --note "Salary" --output json
boring-budget entry add --type expense --amount 12.50 --currency USD --date 2026-02-11 --category-id 1 --label-id 1 --note "Lunch" --output json
boring-budget entry add --type expense --amount 74.25 --currency USD --date 2026-02-11 --note "Coffee" --output json
boring-budget entry add --type expense --amount 95.00 --currency USD --date 2026-02-11 --payment-method card --card-id 1 --note "Groceries" --output json
boring-budget entry list --payment-method credit --from 2026-02-01 --to 2026-02-28 --output json

# Cap management (non-blocking overspend policy)
boring-budget cap set --month 2026-02 --amount 500.00 --currency USD --output json

# Setup with optional decimal onboarding values
boring-budget setup init --default-currency USD --timezone America/New_York --opening-balance 1000.00 --month-cap 500.00 --output json

# Card management and debt tracking
boring-budget card add --nickname "Main Credit" --last4 1234 --brand VISA --card-type credit --due-day 15 --description "Primary card" --output json
boring-budget card list --output json
boring-budget card update 1 --nickname "Main Visa" --output json
boring-budget card due show --card-id 1 --as-of 2026-02-10 --output json
boring-budget card debt show --card-id 1 --output json
boring-budget card payment add --card-id 1 --amount 200.00 --currency USD --note "Statement payment" --output json

# Reporting and balance
boring-budget report monthly --month 2026-02 --group-by month --output json
boring-budget report range --from 2026-02-01 --to 2026-02-28 --payment-method card --card-id 1 --output json
boring-budget balance show --scope both --from 2026-02-01 --to 2026-02-28 --output json

# Portability
boring-budget data export --resource entries --format json --file /tmp/entries.json --output json
boring-budget data backup --file /tmp/boring-budget.db --output json
```

## Determinism checklist

- Use explicit date windows for reports and exports.
- Use stable label filtering mode when provided: `any|all|none`.
- Prefer fixed test dates instead of relative terms.
- Do not depend on object key order; do depend on documented schema.

## Contract references

- Repository mode references:
  - JSON contracts: `docs/contracts/README.md`
  - Example payloads: `docs/contracts/*.json`
  - Error catalog: `docs/contracts/errors.md`
  - Exit codes: `docs/contracts/exit-codes.md`
- Binary-only fallback:
  - `boring-budget --help` for available commands and flags
  - infer errors from `error.code` and process exit code
  - use this stable exit map: `0=success`, `1=internal`, `2=invalid-argument`, `3=not-found`, `4=conflict`, `5=db-error`, `6=external-dependency`, `7=config-error`
- Task-specific playbook: `{baseDir}/references/workflows.md`
