---
summary: Stable error and warning code catalog used in JSON envelopes.
read_when:
  - Adding or changing error codes.
  - Mapping failures/warnings to deterministic agent behavior.
---

# Error Code Catalog (Stable)

Use these codes in `error.code` when `ok=false`.

| code | meaning | typical exit code |
| --- | --- | --- |
| `INVALID_ARGUMENT` | Required field missing or malformed value. | `2` |
| `INVALID_DATE_RANGE` | Date window is invalid (`from > to`, bad preset, etc.). | `2` |
| `INVALID_CURRENCY_CODE` | Currency code is not a supported ISO code. | `2` |
| `NOT_FOUND` | Requested entity does not exist (entry/category/label/cap). | `3` |
| `CONFLICT` | Write conflict, duplicate unique value, or stale update. | `4` |
| `DB_ERROR` | SQLite operation failed. | `5` |
| `FX_RATE_UNAVAILABLE` | Required FX rate could not be resolved. | `6` |
| `CONFIG_ERROR` | Missing/invalid app settings (currency/timezone/onboarding). | `7` |
| `INTERNAL_ERROR` | Unexpected internal failure. | `1` |

Error object shape:

```json
{
  "code": "INVALID_ARGUMENT",
  "message": "transaction_date_utc is required",
  "details": {
    "field": "transaction_date_utc"
  }
}
```

## Warning Code Catalog (Stable)

Use these codes in `warnings[]` when `ok=true` (or alongside non-fatal results).

| code | meaning |
| --- | --- |
| `CAP_EXCEEDED` | Expense was saved and monthly cap is now exceeded. |
| `ORPHAN_COUNT_THRESHOLD_EXCEEDED` | Orphan entry count is above configured threshold. |
| `ORPHAN_SPENDING_THRESHOLD_EXCEEDED` | Orphan spending is above configured threshold. |
| `FX_ESTIMATE_USED` | Future-dated conversion used latest available rate estimate. |
