-- name: CreateLabel :execresult
INSERT INTO labels (name) VALUES (?);

-- name: ListActiveLabels :many
SELECT id, name, created_at_utc, updated_at_utc, deleted_at_utc
FROM labels
WHERE deleted_at_utc IS NULL
ORDER BY lower(name), id;

-- name: GetActiveLabelByID :one
SELECT id, name, created_at_utc, updated_at_utc, deleted_at_utc
FROM labels
WHERE id = ? AND deleted_at_utc IS NULL;

-- name: RenameActiveLabel :execresult
UPDATE labels
SET name = ?, updated_at_utc = ?
WHERE id = ? AND deleted_at_utc IS NULL;

-- name: SoftDeleteLabel :execresult
UPDATE labels
SET deleted_at_utc = ?, updated_at_utc = ?
WHERE id = ? AND deleted_at_utc IS NULL;

-- name: SoftDeleteTransactionLabelLinksByLabelID :execresult
UPDATE transaction_labels
SET deleted_at_utc = ?
WHERE label_id = ? AND deleted_at_utc IS NULL;
