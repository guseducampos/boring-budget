-- +goose Up
-- +goose StatementBegin

ALTER TABLE transactions
    ADD COLUMN bank_account_id INTEGER REFERENCES bank_accounts(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_transactions_bank_account_date
    ON transactions (bank_account_id, transaction_date_utc, id)
    WHERE deleted_at_utc IS NULL AND bank_account_id IS NOT NULL;

ALTER TABLE savings_events
    ADD COLUMN source_bank_account_id INTEGER REFERENCES bank_accounts(id) ON DELETE SET NULL;

ALTER TABLE savings_events
    ADD COLUMN destination_bank_account_id INTEGER REFERENCES bank_accounts(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_savings_events_source_account_date
    ON savings_events (source_bank_account_id, event_date_utc, id)
    WHERE source_bank_account_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_savings_events_destination_account_date
    ON savings_events (destination_bank_account_id, event_date_utc, id)
    WHERE destination_bank_account_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_savings_events_destination_account_date;
DROP INDEX IF EXISTS idx_savings_events_source_account_date;
ALTER TABLE savings_events DROP COLUMN destination_bank_account_id;
ALTER TABLE savings_events DROP COLUMN source_bank_account_id;

DROP INDEX IF EXISTS idx_transactions_bank_account_date;
ALTER TABLE transactions DROP COLUMN bank_account_id;

-- +goose StatementEnd
