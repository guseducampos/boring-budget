-- name: CreateMonthlyCap :execresult
INSERT INTO monthly_caps (
    month_key,
    amount_minor,
    currency_code,
    updated_at_utc
) VALUES (?, ?, ?, ?);

-- name: UpdateMonthlyCapByMonthKey :execresult
UPDATE monthly_caps
SET amount_minor = ?, currency_code = ?, updated_at_utc = ?
WHERE month_key = ?;

-- name: GetMonthlyCapByMonthKey :one
SELECT id, month_key, amount_minor, currency_code, created_at_utc, updated_at_utc
FROM monthly_caps
WHERE month_key = ?;

-- name: CreateMonthlyCapChange :execresult
INSERT INTO monthly_cap_changes (
    month_key,
    old_amount_minor,
    new_amount_minor,
    currency_code,
    changed_at_utc
) VALUES (?, ?, ?, ?, ?);

-- name: ListMonthlyCapChangesByMonthKey :many
SELECT id, month_key, old_amount_minor, new_amount_minor, currency_code, changed_at_utc
FROM monthly_cap_changes
WHERE month_key = ?
ORDER BY changed_at_utc, id;

-- name: SumActiveExpensesByMonthAndCurrency :one
SELECT CAST(COALESCE(SUM(amount_minor), 0) AS INTEGER) AS total_amount_minor
FROM transactions
WHERE type = 'expense'
  AND deleted_at_utc IS NULL
  AND currency_code = ?
  AND transaction_date_utc >= ?
  AND transaction_date_utc < ?;
