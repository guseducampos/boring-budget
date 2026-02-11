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
  AND (sqlc.narg(note_contains) IS NULL OR (note IS NOT NULL AND instr(lower(note), lower(sqlc.narg(note_contains))) > 0))
ORDER BY transaction_date_utc, id;

-- name: SoftDeleteEntry :execresult
UPDATE transactions
SET deleted_at_utc = ?, updated_at_utc = ?
WHERE id = ? AND deleted_at_utc IS NULL;

-- name: UpdateEntryByID :execresult
UPDATE transactions
SET type = CASE
    WHEN sqlc.arg(set_type) = 1 THEN sqlc.arg(type)
    ELSE type
END,
    amount_minor = CASE
    WHEN sqlc.arg(set_amount_minor) = 1 THEN sqlc.arg(amount_minor)
    ELSE amount_minor
END,
    currency_code = CASE
    WHEN sqlc.arg(set_currency_code) = 1 THEN sqlc.arg(currency_code)
    ELSE currency_code
END,
    transaction_date_utc = CASE
    WHEN sqlc.arg(set_transaction_date_utc) = 1 THEN sqlc.arg(transaction_date_utc)
    ELSE transaction_date_utc
END,
    category_id = CASE
    WHEN sqlc.arg(clear_category) = 1 THEN NULL
    WHEN sqlc.arg(set_category_id) = 1 THEN sqlc.narg(category_id)
    ELSE category_id
END,
    note = CASE
    WHEN sqlc.arg(clear_note) = 1 THEN NULL
    WHEN sqlc.arg(set_note) = 1 THEN sqlc.narg(note)
    ELSE note
END,
    updated_at_utc = sqlc.arg(updated_at_utc)
WHERE id = sqlc.arg(id)
  AND deleted_at_utc IS NULL;

-- name: AddEntryLabelLink :execresult
INSERT INTO transaction_labels (transaction_id, label_id)
VALUES (?, ?);

-- name: ListActiveEntryLabelIDs :many
SELECT transaction_id, label_id
FROM transaction_labels
WHERE transaction_id = ? AND deleted_at_utc IS NULL
ORDER BY label_id;

-- name: ListActiveEntryLabelIDsForListFilter :many
SELECT tl.transaction_id, tl.label_id
FROM transaction_labels tl
INNER JOIN transactions t ON t.id = tl.transaction_id
WHERE tl.deleted_at_utc IS NULL
  AND t.deleted_at_utc IS NULL
  AND (sqlc.narg(entry_type) IS NULL OR t.type = sqlc.narg(entry_type))
  AND (sqlc.narg(category_id) IS NULL OR t.category_id = sqlc.narg(category_id))
  AND (sqlc.narg(date_from_utc) IS NULL OR t.transaction_date_utc >= sqlc.narg(date_from_utc))
  AND (sqlc.narg(date_to_utc) IS NULL OR t.transaction_date_utc <= sqlc.narg(date_to_utc))
  AND (sqlc.narg(note_contains) IS NULL OR (t.note IS NOT NULL AND instr(lower(t.note), lower(sqlc.narg(note_contains))) > 0))
ORDER BY tl.transaction_id, tl.label_id;

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
