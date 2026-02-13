-- name: CreateCard :execresult
INSERT INTO cards (
    nickname,
    description,
    last4,
    brand,
    card_type,
    due_day,
    updated_at_utc
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetCardByID :one
SELECT id, nickname, description, last4, brand, card_type, due_day, created_at_utc, updated_at_utc, deleted_at_utc
FROM cards
WHERE id = ?;

-- name: GetActiveCardByID :one
SELECT id, nickname, description, last4, brand, card_type, due_day, created_at_utc, updated_at_utc, deleted_at_utc
FROM cards
WHERE id = ?
  AND deleted_at_utc IS NULL;

-- name: GetActiveCardByNickname :one
SELECT id, nickname, description, last4, brand, card_type, due_day, created_at_utc, updated_at_utc, deleted_at_utc
FROM cards
WHERE lower(nickname) = lower(?)
  AND deleted_at_utc IS NULL;

-- name: ListCards :many
SELECT id, nickname, description, last4, brand, card_type, due_day, created_at_utc, updated_at_utc, deleted_at_utc
FROM cards
WHERE (sqlc.arg(include_deleted) = 1 OR deleted_at_utc IS NULL)
  AND (sqlc.narg(card_type) IS NULL OR card_type = sqlc.narg(card_type))
ORDER BY lower(nickname), id;

-- name: SearchActiveCardsByLookup :many
SELECT id, nickname, description, last4, brand, card_type, due_day, created_at_utc, updated_at_utc, deleted_at_utc
FROM cards
WHERE deleted_at_utc IS NULL
  AND (
      instr(lower(nickname), lower(sqlc.arg(lookup_text))) > 0
      OR (description IS NOT NULL AND instr(lower(description), lower(sqlc.arg(lookup_text))) > 0)
      OR instr(last4, sqlc.arg(lookup_text)) > 0
  )
ORDER BY lower(nickname), id
LIMIT sqlc.arg(limit_rows);

-- name: UpdateCardByID :execresult
UPDATE cards
SET nickname = CASE
    WHEN sqlc.arg(set_nickname) = 1 THEN sqlc.arg(nickname)
    ELSE nickname
END,
    description = CASE
    WHEN sqlc.arg(clear_description) = 1 THEN NULL
    WHEN sqlc.arg(set_description) = 1 THEN sqlc.narg(description)
    ELSE description
END,
    last4 = CASE
    WHEN sqlc.arg(set_last4) = 1 THEN sqlc.arg(last4)
    ELSE last4
END,
    brand = CASE
    WHEN sqlc.arg(set_brand) = 1 THEN sqlc.arg(brand)
    ELSE brand
END,
    card_type = CASE
    WHEN sqlc.arg(set_card_type) = 1 THEN sqlc.arg(card_type)
    ELSE card_type
END,
    due_day = CASE
    WHEN sqlc.arg(clear_due_day) = 1 THEN NULL
    WHEN sqlc.arg(set_due_day) = 1 THEN sqlc.narg(due_day)
    ELSE due_day
END,
    updated_at_utc = sqlc.arg(updated_at_utc)
WHERE id = sqlc.arg(id)
  AND deleted_at_utc IS NULL;

-- name: SoftDeleteCard :execresult
UPDATE cards
SET deleted_at_utc = ?,
    updated_at_utc = ?
WHERE id = ?
  AND deleted_at_utc IS NULL;

-- name: ExistsCardByID :one
SELECT EXISTS(
    SELECT 1
    FROM cards
    WHERE id = ?
);

-- name: ExistsActiveCardByID :one
SELECT EXISTS(
    SELECT 1
    FROM cards
    WHERE id = ?
      AND deleted_at_utc IS NULL
);

-- name: GetActiveCardDueByID :one
SELECT id, nickname, due_day
FROM cards
WHERE id = ?
  AND deleted_at_utc IS NULL
  AND card_type = 'credit';

-- name: ListActiveCreditCardDues :many
SELECT id, nickname, due_day
FROM cards
WHERE deleted_at_utc IS NULL
  AND card_type = 'credit'
ORDER BY due_day, id;

-- name: ExistsTransactionByID :one
SELECT EXISTS(
    SELECT 1
    FROM transactions
    WHERE id = ?
);

-- name: UpsertTransactionPaymentMethod :execresult
INSERT INTO transaction_payment_methods (
    transaction_id,
    method_type,
    card_id,
    updated_at_utc
) VALUES (?, ?, ?, ?)
ON CONFLICT(transaction_id)
DO UPDATE SET
    method_type = excluded.method_type,
    card_id = excluded.card_id,
    updated_at_utc = excluded.updated_at_utc;

-- name: GetTransactionPaymentMethodByTransactionID :one
SELECT transaction_id, method_type, card_id, created_at_utc, updated_at_utc
FROM transaction_payment_methods
WHERE transaction_id = ?;

-- name: CreateCreditLiabilityEvent :execresult
INSERT INTO credit_liability_events (
    card_id,
    currency_code,
    event_type,
    amount_minor_signed,
    reference_transaction_id,
    note,
    created_at_utc
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetCreditLiabilityEventByID :one
SELECT id, card_id, currency_code, event_type, amount_minor_signed, reference_transaction_id, note, created_at_utc
FROM credit_liability_events
WHERE id = ?;

-- name: ListCreditLiabilityEventsByCard :many
SELECT id, card_id, currency_code, event_type, amount_minor_signed, reference_transaction_id, note, created_at_utc
FROM credit_liability_events
WHERE card_id = ?
ORDER BY currency_code, created_at_utc, id;

-- name: ListCreditLiabilityEventsByCardAndCurrency :many
SELECT id, card_id, currency_code, event_type, amount_minor_signed, reference_transaction_id, note, created_at_utc
FROM credit_liability_events
WHERE card_id = ?
  AND currency_code = ?
ORDER BY created_at_utc, id;

-- name: GetCreditLiabilityBalanceByCardAndCurrency :one
SELECT CAST(COALESCE(SUM(amount_minor_signed), 0) AS INTEGER) AS balance_minor
FROM credit_liability_events
WHERE card_id = ?
  AND currency_code = ?;

-- name: ListCreditLiabilitySummaryByCard :many
SELECT card_id,
       currency_code,
       CAST(COALESCE(SUM(amount_minor_signed), 0) AS INTEGER) AS balance_minor,
       MAX(created_at_utc) AS last_event_at_utc
FROM credit_liability_events
WHERE card_id = ?
GROUP BY card_id, currency_code
ORDER BY currency_code;

-- name: ListCreditLiabilitySummaryAllCards :many
SELECT card_id,
       currency_code,
       CAST(COALESCE(SUM(amount_minor_signed), 0) AS INTEGER) AS balance_minor,
       MAX(created_at_utc) AS last_event_at_utc
FROM credit_liability_events
GROUP BY card_id, currency_code
ORDER BY card_id, currency_code;

-- name: DeleteCreditLiabilityEventsByReferenceTransaction :execresult
DELETE FROM credit_liability_events
WHERE reference_transaction_id = ?;
