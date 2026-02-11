-- name: UpsertSettings :execresult
INSERT INTO settings (
    id,
    default_currency_code,
    display_timezone,
    orphan_count_threshold,
    orphan_spending_threshold_bps,
    onboarding_completed_at_utc,
    updated_at_utc
) VALUES (1, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    default_currency_code = excluded.default_currency_code,
    display_timezone = excluded.display_timezone,
    orphan_count_threshold = excluded.orphan_count_threshold,
    orphan_spending_threshold_bps = excluded.orphan_spending_threshold_bps,
    onboarding_completed_at_utc = excluded.onboarding_completed_at_utc,
    updated_at_utc = excluded.updated_at_utc;

-- name: GetSettings :one
SELECT id,
       default_currency_code,
       display_timezone,
       orphan_count_threshold,
       orphan_spending_threshold_bps,
       onboarding_completed_at_utc,
       created_at_utc,
       updated_at_utc
FROM settings
WHERE id = 1;
