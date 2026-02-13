-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS cards (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nickname TEXT NOT NULL,
    description TEXT,
    last4 TEXT NOT NULL CHECK (length(last4) = 4 AND last4 GLOB '[0-9][0-9][0-9][0-9]'),
    brand TEXT NOT NULL,
    card_type TEXT NOT NULL CHECK (card_type IN ('credit', 'debit')),
    due_day INTEGER CHECK (due_day BETWEEN 1 AND 28),
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at_utc TEXT,
    CHECK (
        (card_type = 'credit' AND due_day IS NOT NULL) OR
        (card_type = 'debit' AND due_day IS NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cards_nickname_active
    ON cards (lower(nickname))
    WHERE deleted_at_utc IS NULL;

CREATE INDEX IF NOT EXISTS idx_cards_type_active
    ON cards (card_type, deleted_at_utc);

CREATE TABLE IF NOT EXISTS transaction_payment_methods (
    transaction_id INTEGER PRIMARY KEY REFERENCES transactions(id) ON DELETE CASCADE,
    method_type TEXT NOT NULL CHECK (method_type IN ('cash', 'card')),
    card_id INTEGER REFERENCES cards(id) ON DELETE RESTRICT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK (
        (method_type = 'cash' AND card_id IS NULL) OR
        (method_type = 'card' AND card_id IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_transaction_payment_methods_method_card
    ON transaction_payment_methods (method_type, card_id);

CREATE INDEX IF NOT EXISTS idx_transaction_payment_methods_card
    ON transaction_payment_methods (card_id)
    WHERE card_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS credit_liability_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    card_id INTEGER NOT NULL REFERENCES cards(id) ON DELETE RESTRICT,
    currency_code TEXT NOT NULL CHECK (length(currency_code) = 3),
    event_type TEXT NOT NULL CHECK (event_type IN ('charge', 'payment', 'adjustment')),
    amount_minor_signed INTEGER NOT NULL,
    reference_transaction_id INTEGER REFERENCES transactions(id) ON DELETE SET NULL,
    note TEXT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK (
        (event_type = 'charge' AND amount_minor_signed > 0) OR
        (event_type = 'payment' AND amount_minor_signed < 0) OR
        (event_type = 'adjustment' AND amount_minor_signed != 0)
    )
);

CREATE INDEX IF NOT EXISTS idx_credit_liability_events_card_currency_time
    ON credit_liability_events (card_id, currency_code, created_at_utc, id);

CREATE INDEX IF NOT EXISTS idx_credit_liability_events_reference_transaction
    ON credit_liability_events (reference_transaction_id)
    WHERE reference_transaction_id IS NOT NULL;

INSERT INTO transaction_payment_methods (transaction_id, method_type, card_id)
SELECT t.id, 'cash', NULL
FROM transactions t
WHERE t.type = 'expense'
  AND NOT EXISTS (
      SELECT 1
      FROM transaction_payment_methods pm
      WHERE pm.transaction_id = t.id
  );

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_credit_liability_events_reference_transaction;
DROP INDEX IF EXISTS idx_credit_liability_events_card_currency_time;
DROP TABLE IF EXISTS credit_liability_events;

DROP INDEX IF EXISTS idx_transaction_payment_methods_card;
DROP INDEX IF EXISTS idx_transaction_payment_methods_method_card;
DROP TABLE IF EXISTS transaction_payment_methods;

DROP INDEX IF EXISTS idx_cards_type_active;
DROP INDEX IF EXISTS idx_cards_nickname_active;
DROP TABLE IF EXISTS cards;

-- +goose StatementEnd
