# Workflow Playbook

Use this reference when executing common `boring-budget` tasks repeatedly.

## 0) Preflight for binary-only agents

1. Check command availability:
   - `command -v boring-budget`
2. If missing, install:
   - `brew install guseducampos/tap/boring-budget`
3. Verify command surface:
   - `boring-budget --help`
4. Assume repo docs may be unavailable; rely on runtime JSON envelopes.

## 1) First-run bootstrap

1. Check setup:
   - `boring-budget setup show --output json`
2. If setup is missing, initialize:
   - `boring-budget setup init --default-currency USD --timezone America/New_York --output json`
3. Verify envelope:
   - `ok=true`
   - `error=null`

## 2) Add and review entries

1. Add supporting taxonomy:
   - `category add`, `label add`
2. Add entries with explicit required fields:
   - `--type`, `--amount-minor`, `--currency`, `--date`
3. Query back with filters:
   - `entry list --from ... --to ... --label-mode any|all|none --output json`
4. Validate:
   - amounts are integers in minor units
   - timestamps are UTC values

## 3) Cap-safe expense writes

1. Set or update cap:
   - `cap set --month YYYY-MM --amount-minor ... --currency ... --output json`
2. Add/update expense entry.
3. If `warnings[]` contains `CAP_EXCEEDED`, treat as successful write plus warning.
4. Confirm cap history:
   - `cap history --month YYYY-MM --output json`

## 4) Reporting and balance flows

1. Report by period:
   - `report range|monthly|bimonthly|quarterly ... --output json`
2. Keep grouping explicit:
   - `--group-by day|week|month`
3. Keep filter semantics explicit:
   - dates, category, labels, `--label-mode`
4. Balance:
   - `balance show --scope lifetime|range|both ... --output json`

## 5) Data portability and recovery

1. Export:
   - `data export --resource entries|report --format json|csv --file ... --output json`
2. Import:
   - `data import --format json|csv --file ... [--idempotent] --output json`
3. Backup/restore:
   - `data backup --file ... --output json`
   - `data restore --file ... --output json`
4. After restore, verify with:
   - `report monthly --month YYYY-MM --output json`

## 6) Error and exit handling

- Repository mode source of truth:
  - `docs/contracts/errors.md`
  - `docs/contracts/exit-codes.md`
- Binary-only fallback:
  - parse `error.code` from JSON envelope
  - use exit mapping `0..7` from skill summary
- Routing rules:
  - `INVALID_ARGUMENT`, `INVALID_DATE_RANGE`, `INVALID_CURRENCY_CODE` -> correct request payload
  - `NOT_FOUND` -> verify IDs/month keys and retry
  - `CONFLICT` -> refresh state then retry write
  - `DB_ERROR`, `INTERNAL_ERROR` -> stop and surface failure context
