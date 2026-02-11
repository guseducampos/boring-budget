-- name: CreateCategory :execresult
INSERT INTO categories (name) VALUES (?);

-- name: ListActiveCategories :many
SELECT id, name, created_at_utc, updated_at_utc
FROM categories
WHERE deleted_at_utc IS NULL
ORDER BY lower(name), id;

-- name: GetActiveCategoryByID :one
SELECT id, name, created_at_utc, updated_at_utc
FROM categories
WHERE id = ? AND deleted_at_utc IS NULL;

-- name: RenameActiveCategory :execresult
UPDATE categories
SET name = ?, updated_at_utc = ?
WHERE id = ? AND deleted_at_utc IS NULL;

-- name: SoftDeleteCategory :execresult
UPDATE categories
SET deleted_at_utc = ?, updated_at_utc = ?
WHERE id = ? AND deleted_at_utc IS NULL;

-- name: OrphanActiveTransactionsByCategoryID :execresult
UPDATE transactions
SET category_id = NULL, updated_at_utc = ?
WHERE category_id = ? AND deleted_at_utc IS NULL;
