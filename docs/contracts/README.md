# Budgetto Agent Contracts

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
- Keep top-level key order exactly: `ok`, `data`, `warnings`, `error`, `meta`.
- Store and return money in minor units only (`amount_minor`) plus `currency_code`.
- Use ISO-8601 UTC timestamps (`...Z`).
- Keep arrays deterministically ordered (typically by date, then ID).
- `error` is `null` on success, object on failure: `{ "code", "message", "details" }`.

## Files

- `entry-add.json`: `entry add --output json` success contract with overspend warning.
- `entry-update.json`: `entry update --output json` success contract.
- `cap-set.json`: `cap set --output json` success contract with cap history change.
- `report-monthly.json`: `report monthly --output json` success contract.
- `setup-init.json`: `setup init --output json` onboarding success contract.
- `data-export.json`: `data export --output json` success contract.
- `errors.md`: stable error and warning code catalog.
- `exit-codes.md`: stable CLI exit-code table.
