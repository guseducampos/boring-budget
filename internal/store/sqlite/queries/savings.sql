-- name: CreateSavingsEvent :execresult
INSERT INTO savings_events (
    event_type,
    amount_minor,
    currency_code,
    event_date_utc,
    note
) VALUES (?, ?, ?, ?, ?);

-- name: ListSavingsEvents :many
SELECT id, event_type, amount_minor, currency_code, event_date_utc, note, created_at_utc
FROM savings_events
WHERE (sqlc.narg(date_from_utc) IS NULL OR event_date_utc >= sqlc.narg(date_from_utc))
  AND (sqlc.narg(date_to_utc) IS NULL OR event_date_utc <= sqlc.narg(date_to_utc))
  AND (sqlc.narg(event_type) IS NULL OR event_type = sqlc.narg(event_type))
ORDER BY event_date_utc, id;
