-- name: CreateEntry :execresult
INSERT INTO transactions (
    type,
    amount_minor,
    currency_code,
    transaction_date_utc,
    category_id,
    note
) VALUES (?, ?, ?, ?, ?, ?);

-- name: GetActiveEntryByID :one
SELECT id, type, amount_minor, currency_code, transaction_date_utc, category_id, note, created_at_utc, updated_at_utc, deleted_at_utc
FROM transactions
WHERE id = ? AND deleted_at_utc IS NULL;

-- name: ListActiveEntries :many
SELECT id, type, amount_minor, currency_code, transaction_date_utc, category_id, note, created_at_utc, updated_at_utc, deleted_at_utc
FROM transactions
WHERE deleted_at_utc IS NULL
  AND (sqlc.narg(entry_type) IS NULL OR type = sqlc.narg(entry_type))
  AND (sqlc.narg(category_id) IS NULL OR category_id = sqlc.narg(category_id))
  AND (sqlc.narg(date_from_utc) IS NULL OR transaction_date_utc >= sqlc.narg(date_from_utc))
  AND (sqlc.narg(date_to_utc) IS NULL OR transaction_date_utc <= sqlc.narg(date_to_utc))
ORDER BY transaction_date_utc, id;

-- name: SoftDeleteEntry :execresult
UPDATE transactions
SET deleted_at_utc = ?, updated_at_utc = ?
WHERE id = ? AND deleted_at_utc IS NULL;

-- name: AddEntryLabelLink :execresult
INSERT INTO transaction_labels (transaction_id, label_id)
VALUES (?, ?);

-- name: ListActiveEntryLabelIDs :many
SELECT transaction_id, label_id
FROM transaction_labels
WHERE transaction_id = ? AND deleted_at_utc IS NULL
ORDER BY label_id;

-- name: SoftDeleteEntryLabelLinks :execresult
UPDATE transaction_labels
SET deleted_at_utc = ?
WHERE transaction_id = ? AND deleted_at_utc IS NULL;

-- name: ExistsActiveCategoryByID :one
SELECT EXISTS(
    SELECT 1
    FROM categories
    WHERE id = ? AND deleted_at_utc IS NULL
);

-- name: ExistsActiveLabelByID :one
SELECT EXISTS(
    SELECT 1
    FROM labels
    WHERE id = ? AND deleted_at_utc IS NULL
);
