CREATE TABLE IF NOT EXISTS categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at_utc TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_categories_name_active
    ON categories (lower(name))
    WHERE deleted_at_utc IS NULL;

CREATE TABLE IF NOT EXISTS labels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at_utc TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_labels_name_active
    ON labels (lower(name))
    WHERE deleted_at_utc IS NULL;

CREATE TABLE IF NOT EXISTS transactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL CHECK (type IN ('income', 'expense')),
    amount_minor INTEGER NOT NULL,
    currency_code TEXT NOT NULL CHECK (length(currency_code) = 3),
    transaction_date_utc TEXT NOT NULL,
    category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    note TEXT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at_utc TEXT
);

CREATE INDEX IF NOT EXISTS idx_transactions_date
    ON transactions (transaction_date_utc);

CREATE INDEX IF NOT EXISTS idx_transactions_type
    ON transactions (type);

CREATE INDEX IF NOT EXISTS idx_transactions_category
    ON transactions (category_id);

CREATE INDEX IF NOT EXISTS idx_transactions_deleted_date
    ON transactions (deleted_at_utc, transaction_date_utc);

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

CREATE TABLE IF NOT EXISTS transaction_labels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    label_id INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at_utc TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_transaction_labels_unique_active
    ON transaction_labels (transaction_id, label_id)
    WHERE deleted_at_utc IS NULL;

CREATE INDEX IF NOT EXISTS idx_transaction_labels_label_active
    ON transaction_labels (label_id, deleted_at_utc);

CREATE INDEX IF NOT EXISTS idx_transaction_labels_transaction_active
    ON transaction_labels (transaction_id, deleted_at_utc);

CREATE TABLE IF NOT EXISTS monthly_caps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    month_key TEXT NOT NULL,
    amount_minor INTEGER NOT NULL,
    currency_code TEXT NOT NULL CHECK (length(currency_code) = 3),
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_monthly_caps_month
    ON monthly_caps (month_key);

CREATE TABLE IF NOT EXISTS monthly_cap_changes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    month_key TEXT NOT NULL,
    old_amount_minor INTEGER,
    new_amount_minor INTEGER NOT NULL,
    currency_code TEXT NOT NULL CHECK (length(currency_code) = 3),
    changed_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_monthly_cap_changes_month_changed
    ON monthly_cap_changes (month_key, changed_at_utc);

CREATE TABLE IF NOT EXISTS settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    default_currency_code TEXT NOT NULL CHECK (length(default_currency_code) = 3),
    display_timezone TEXT NOT NULL,
    orphan_count_threshold INTEGER NOT NULL DEFAULT 5,
    orphan_spending_threshold_bps INTEGER NOT NULL DEFAULT 500,
    onboarding_completed_at_utc TEXT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS fx_rate_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL,
    base_currency TEXT NOT NULL CHECK (length(base_currency) = 3),
    quote_currency TEXT NOT NULL CHECK (length(quote_currency) = 3),
    rate TEXT NOT NULL,
    rate_date TEXT NOT NULL,
    is_estimate INTEGER NOT NULL DEFAULT 0 CHECK (is_estimate IN (0, 1)),
    fetched_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_fx_rate_snapshot_unique
    ON fx_rate_snapshots (provider, base_currency, quote_currency, rate_date, is_estimate);

CREATE TABLE IF NOT EXISTS audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    source TEXT NOT NULL,
    payload_json TEXT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_audit_events_entity_time
    ON audit_events (entity_type, entity_id, created_at_utc);
