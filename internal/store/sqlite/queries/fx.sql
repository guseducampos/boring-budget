-- name: GetFXRateSnapshotByKey :one
SELECT id,
       provider,
       base_currency,
       quote_currency,
       rate,
       rate_date,
       is_estimate,
       fetched_at_utc
FROM fx_rate_snapshots
WHERE provider = ?
  AND base_currency = ?
  AND quote_currency = ?
  AND rate_date = ?
  AND is_estimate = ?;

-- name: CreateFXRateSnapshot :execresult
INSERT INTO fx_rate_snapshots (
    provider,
    base_currency,
    quote_currency,
    rate,
    rate_date,
    is_estimate,
    fetched_at_utc
) VALUES (?, ?, ?, ?, ?, ?, ?);
