-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS savings_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL CHECK (event_type IN ('transfer_to_savings', 'independent_add')),
    amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
    currency_code TEXT NOT NULL CHECK (length(currency_code) = 3),
    event_date_utc TEXT NOT NULL,
    note TEXT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_savings_events_date_currency
    ON savings_events (event_date_utc, currency_code, id);

CREATE INDEX IF NOT EXISTS idx_savings_events_type_date
    ON savings_events (event_type, event_date_utc, id);

CREATE TABLE IF NOT EXISTS bank_accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    alias TEXT NOT NULL,
    last4 TEXT NOT NULL CHECK (length(last4) = 4 AND last4 GLOB '[0-9][0-9][0-9][0-9]'),
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at_utc TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bank_accounts_alias_active
    ON bank_accounts (lower(alias))
    WHERE deleted_at_utc IS NULL;

CREATE TABLE IF NOT EXISTS balance_account_links (
    target TEXT PRIMARY KEY CHECK (target IN ('general_balance', 'savings')),
    bank_account_id INTEGER REFERENCES bank_accounts(id) ON DELETE SET NULL,
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS scheduled_payments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
    currency_code TEXT NOT NULL CHECK (length(currency_code) = 3),
    day_of_month INTEGER NOT NULL CHECK (day_of_month BETWEEN 1 AND 28),
    start_month_key TEXT NOT NULL,
    end_month_key TEXT,
    category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    note TEXT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at_utc TEXT,
    CHECK (end_month_key IS NULL OR end_month_key >= start_month_key)
);

CREATE INDEX IF NOT EXISTS idx_scheduled_payments_active_day
    ON scheduled_payments (deleted_at_utc, day_of_month, start_month_key, end_month_key, id);

CREATE TABLE IF NOT EXISTS scheduled_payment_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    schedule_id INTEGER NOT NULL REFERENCES scheduled_payments(id) ON DELETE CASCADE,
    month_key TEXT NOT NULL,
    entry_id INTEGER REFERENCES transactions(id) ON DELETE SET NULL,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (schedule_id, month_key)
);

CREATE INDEX IF NOT EXISTS idx_scheduled_payment_executions_schedule
    ON scheduled_payment_executions (schedule_id, month_key);

CREATE INDEX IF NOT EXISTS idx_scheduled_payment_executions_entry
    ON scheduled_payment_executions (entry_id)
    WHERE entry_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_scheduled_payment_executions_entry;
DROP INDEX IF EXISTS idx_scheduled_payment_executions_schedule;
DROP TABLE IF EXISTS scheduled_payment_executions;

DROP INDEX IF EXISTS idx_scheduled_payments_active_day;
DROP TABLE IF EXISTS scheduled_payments;

DROP TABLE IF EXISTS balance_account_links;
DROP INDEX IF EXISTS idx_bank_accounts_alias_active;
DROP TABLE IF EXISTS bank_accounts;

DROP INDEX IF EXISTS idx_savings_events_type_date;
DROP INDEX IF EXISTS idx_savings_events_date_currency;
DROP TABLE IF EXISTS savings_events;

-- +goose StatementEnd
