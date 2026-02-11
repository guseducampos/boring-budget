# Exit Codes (Stable)

`boring-budget` uses the same exit codes for `--output human` and `--output json`.

| code | meaning |
| --- | --- |
| `0` | Success (warnings may exist). |
| `1` | Internal/unexpected error. |
| `2` | Invalid arguments or validation failure. |
| `3` | Requested resource not found. |
| `4` | Conflict/business-state violation. |
| `5` | Database failure (SQLite). |
| `6` | External dependency failure (for example FX provider). |
| `7` | Configuration/onboarding error. |

Notes:
- Warnings never change exit code when command succeeds.
- For `ok=false`, map `error.code` from `errors.md` to the table above.
