---
summary: Deterministic JSON contract guide and example index for agent integrations.
read_when:
  - Changing JSON output structure, warnings, or envelope fields.
  - Building or fixing agent flows that parse boring-budget responses.
---

# boring-budget Agent Contracts

This folder defines deterministic JSON contract examples for agent integrations.

## Envelope (all JSON responses)

```json
{
  "ok": true,
  "data": {},
  "warnings": [],
  "error": null,
  "meta": {
    "api_version": "v1",
    "timestamp_utc": "2026-02-10T00:00:00Z"
  }
}
```

Rules:
- Fixtures are canonicalized in tests before comparison; JSON object key order is not semantically relevant.
- Store and return money in minor units only (`amount_minor`) plus `currency_code`.
- Human-facing commands may accept major-unit input flags (for example, `--amount`), but persisted and JSON contract amounts remain `amount_minor`.
- Use ISO-8601 UTC timestamps (`...Z`).
- Replace volatile timestamps in examples with `<timestamp_utc>`.
- Keep arrays deterministically ordered (typically by date, then ID).
- `error` is `null` on success, object on failure: `{ "code", "message", "details" }`.

## Files

- `entry-add.json`: `entry add --output json` success contract.
- `entry-update.json`: `entry update --output json` success contract.
- `cap-set.json`: `cap set --output json` success contract with cap history change.
- `cap-show.json`: `cap show --output json` success contract.
- `cap-history.json`: `cap history --output json` success contract.
- `report-monthly.json`: `report monthly --output json` success contract.
- `report-range.json`: `report range --output json` success contract.
- `report-bimonthly.json`: `report bimonthly --output json` success contract.
- `report-quarterly.json`: `report quarterly --output json` success contract.
- `balance-show.json`: `balance show --output json` success contract.
- `setup-init.json`: `setup init --output json` onboarding success contract.
- `data-export.json`: `data export --resource entries --output json` success contract.
- `data-export-report.json`: `data export --resource report --output json` success contract.
- `errors.md`: stable error and warning code catalog.
- `exit-codes.md`: stable CLI exit-code table.
