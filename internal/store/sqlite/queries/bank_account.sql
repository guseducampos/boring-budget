-- name: CreateBankAccount :execresult
INSERT INTO bank_accounts (
    alias,
    last4,
    updated_at_utc
) VALUES (?, ?, ?);

-- name: GetBankAccountByID :one
SELECT id, alias, last4, created_at_utc, updated_at_utc, deleted_at_utc
FROM bank_accounts
WHERE id = ?;

-- name: GetActiveBankAccountByID :one
SELECT id, alias, last4, created_at_utc, updated_at_utc, deleted_at_utc
FROM bank_accounts
WHERE id = ? AND deleted_at_utc IS NULL;

-- name: ListBankAccounts :many
SELECT id, alias, last4, created_at_utc, updated_at_utc, deleted_at_utc
FROM bank_accounts
WHERE (sqlc.arg(include_deleted) = 1 OR deleted_at_utc IS NULL)
ORDER BY lower(alias), id;

-- name: SearchActiveBankAccountsByLookup :many
SELECT id, alias, last4, created_at_utc, updated_at_utc, deleted_at_utc
FROM bank_accounts
WHERE deleted_at_utc IS NULL
  AND (
      instr(lower(alias), lower(sqlc.arg(lookup_text))) > 0
      OR instr(last4, sqlc.arg(lookup_text)) > 0
  )
ORDER BY lower(alias), id
LIMIT sqlc.arg(limit_rows);

-- name: UpdateBankAccountByID :execresult
UPDATE bank_accounts
SET alias = CASE
    WHEN sqlc.arg(set_alias) = 1 THEN sqlc.arg(alias)
    ELSE alias
END,
    last4 = CASE
    WHEN sqlc.arg(set_last4) = 1 THEN sqlc.arg(last4)
    ELSE last4
END,
    updated_at_utc = sqlc.arg(updated_at_utc)
WHERE id = sqlc.arg(id)
  AND deleted_at_utc IS NULL;

-- name: SoftDeleteBankAccount :execresult
UPDATE bank_accounts
SET deleted_at_utc = ?, updated_at_utc = ?
WHERE id = ? AND deleted_at_utc IS NULL;

-- name: UpsertBalanceAccountLink :execresult
INSERT INTO balance_account_links (
    target,
    bank_account_id,
    updated_at_utc
) VALUES (sqlc.arg(target), sqlc.narg(bank_account_id), sqlc.arg(updated_at_utc))
ON CONFLICT(target) DO UPDATE SET
    bank_account_id = excluded.bank_account_id,
    updated_at_utc = excluded.updated_at_utc;

-- name: ListBalanceAccountLinks :many
SELECT
    l.target,
    l.bank_account_id,
    l.updated_at_utc,
    b.alias AS account_alias,
    b.last4 AS account_last4
FROM balance_account_links l
LEFT JOIN bank_accounts b
    ON b.id = l.bank_account_id
   AND b.deleted_at_utc IS NULL
ORDER BY l.target;
