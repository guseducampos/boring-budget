-- name: CreateScheduledPayment :execresult
INSERT INTO scheduled_payments (
    name,
    amount_minor,
    currency_code,
    day_of_month,
    start_month_key,
    end_month_key,
    category_id,
    note,
    updated_at_utc
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetActiveScheduledPaymentByID :one
SELECT id, name, amount_minor, currency_code, day_of_month, start_month_key, end_month_key, category_id, note, created_at_utc, updated_at_utc, deleted_at_utc
FROM scheduled_payments
WHERE id = ? AND deleted_at_utc IS NULL;

-- name: ListScheduledPayments :many
SELECT
    sp.id,
    sp.name,
    sp.amount_minor,
    sp.currency_code,
    sp.day_of_month,
    sp.start_month_key,
    sp.end_month_key,
    sp.category_id,
    sp.note,
    sp.created_at_utc,
    sp.updated_at_utc,
    sp.deleted_at_utc,
    (
        SELECT MAX(spe.month_key)
        FROM scheduled_payment_executions spe
        WHERE spe.schedule_id = sp.id
    ) AS last_executed_month_key
FROM scheduled_payments sp
WHERE (sqlc.arg(include_deleted) = 1 OR sp.deleted_at_utc IS NULL)
ORDER BY lower(sp.name), sp.id;

-- name: SoftDeleteScheduledPayment :execresult
UPDATE scheduled_payments
SET deleted_at_utc = ?, updated_at_utc = ?
WHERE id = ? AND deleted_at_utc IS NULL;

-- name: ClaimScheduledPaymentExecution :execresult
INSERT INTO scheduled_payment_executions (
    schedule_id,
    month_key,
    created_at_utc
) VALUES (?, ?, ?);

-- name: AttachScheduledPaymentExecutionEntry :execresult
UPDATE scheduled_payment_executions
SET entry_id = ?
WHERE schedule_id = ? AND month_key = ?;

-- name: ListScheduledPaymentExecutionsByScheduleID :many
SELECT id, schedule_id, month_key, entry_id, created_at_utc
FROM scheduled_payment_executions
WHERE schedule_id = ?
ORDER BY month_key, id;

-- name: ListScheduledPaymentExecutions :many
SELECT id, schedule_id, month_key, entry_id, created_at_utc
FROM scheduled_payment_executions
ORDER BY schedule_id, month_key, id;
