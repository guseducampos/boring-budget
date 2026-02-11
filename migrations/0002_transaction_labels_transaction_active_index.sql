-- +goose Up
CREATE INDEX IF NOT EXISTS idx_transaction_labels_transaction_active
    ON transaction_labels (transaction_id, deleted_at_utc);

-- +goose Down
DROP INDEX IF EXISTS idx_transaction_labels_transaction_active;
