-- +goose Up
-- +goose StatementBegin

CREATE TRIGGER IF NOT EXISTS trg_audit_categories_insert
AFTER INSERT ON categories
BEGIN
    INSERT INTO audit_events (action, entity_type, entity_id, source, payload_json, created_at_utc)
    VALUES (
        'create',
        'category',
        CAST(NEW.id AS TEXT),
        'db_trigger',
        json_object('name', NEW.name),
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_audit_categories_update
AFTER UPDATE ON categories
WHEN NEW.deleted_at_utc IS NULL AND OLD.name IS NOT NEW.name
BEGIN
    INSERT INTO audit_events (action, entity_type, entity_id, source, payload_json, created_at_utc)
    VALUES (
        'update',
        'category',
        CAST(NEW.id AS TEXT),
        'db_trigger',
        json_object('old_name', OLD.name, 'new_name', NEW.name),
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_audit_categories_soft_delete
AFTER UPDATE ON categories
WHEN OLD.deleted_at_utc IS NULL AND NEW.deleted_at_utc IS NOT NULL
BEGIN
    INSERT INTO audit_events (action, entity_type, entity_id, source, payload_json, created_at_utc)
    VALUES (
        'delete',
        'category',
        CAST(NEW.id AS TEXT),
        'db_trigger',
        json_object('name', NEW.name),
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_audit_labels_insert
AFTER INSERT ON labels
BEGIN
    INSERT INTO audit_events (action, entity_type, entity_id, source, payload_json, created_at_utc)
    VALUES (
        'create',
        'label',
        CAST(NEW.id AS TEXT),
        'db_trigger',
        json_object('name', NEW.name),
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_audit_labels_update
AFTER UPDATE ON labels
WHEN NEW.deleted_at_utc IS NULL AND OLD.name IS NOT NEW.name
BEGIN
    INSERT INTO audit_events (action, entity_type, entity_id, source, payload_json, created_at_utc)
    VALUES (
        'update',
        'label',
        CAST(NEW.id AS TEXT),
        'db_trigger',
        json_object('old_name', OLD.name, 'new_name', NEW.name),
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_audit_labels_soft_delete
AFTER UPDATE ON labels
WHEN OLD.deleted_at_utc IS NULL AND NEW.deleted_at_utc IS NOT NULL
BEGIN
    INSERT INTO audit_events (action, entity_type, entity_id, source, payload_json, created_at_utc)
    VALUES (
        'delete',
        'label',
        CAST(NEW.id AS TEXT),
        'db_trigger',
        json_object('name', NEW.name),
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_audit_transactions_insert
AFTER INSERT ON transactions
BEGIN
    INSERT INTO audit_events (action, entity_type, entity_id, source, payload_json, created_at_utc)
    VALUES (
        'create',
        'entry',
        CAST(NEW.id AS TEXT),
        'db_trigger',
        json_object(
            'type', NEW.type,
            'amount_minor', NEW.amount_minor,
            'currency_code', NEW.currency_code,
            'transaction_date_utc', NEW.transaction_date_utc,
            'category_id', NEW.category_id
        ),
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_audit_transactions_soft_delete
AFTER UPDATE ON transactions
WHEN OLD.deleted_at_utc IS NULL AND NEW.deleted_at_utc IS NOT NULL
BEGIN
    INSERT INTO audit_events (action, entity_type, entity_id, source, payload_json, created_at_utc)
    VALUES (
        'delete',
        'entry',
        CAST(NEW.id AS TEXT),
        'db_trigger',
        json_object('deleted_at_utc', NEW.deleted_at_utc),
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_audit_monthly_caps_insert
AFTER INSERT ON monthly_caps
BEGIN
    INSERT INTO audit_events (action, entity_type, entity_id, source, payload_json, created_at_utc)
    VALUES (
        'create',
        'monthly_cap',
        CAST(NEW.id AS TEXT),
        'db_trigger',
        json_object(
            'month_key', NEW.month_key,
            'amount_minor', NEW.amount_minor,
            'currency_code', NEW.currency_code
        ),
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_audit_monthly_caps_update
AFTER UPDATE ON monthly_caps
BEGIN
    INSERT INTO audit_events (action, entity_type, entity_id, source, payload_json, created_at_utc)
    VALUES (
        'update',
        'monthly_cap',
        CAST(NEW.id AS TEXT),
        'db_trigger',
        json_object(
            'month_key', NEW.month_key,
            'old_amount_minor', OLD.amount_minor,
            'new_amount_minor', NEW.amount_minor,
            'currency_code', NEW.currency_code
        ),
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_audit_settings_insert
AFTER INSERT ON settings
BEGIN
    INSERT INTO audit_events (action, entity_type, entity_id, source, payload_json, created_at_utc)
    VALUES (
        'create',
        'settings',
        CAST(NEW.id AS TEXT),
        'db_trigger',
        json_object(
            'default_currency_code', NEW.default_currency_code,
            'display_timezone', NEW.display_timezone
        ),
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_audit_settings_update
AFTER UPDATE ON settings
BEGIN
    INSERT INTO audit_events (action, entity_type, entity_id, source, payload_json, created_at_utc)
    VALUES (
        'update',
        'settings',
        CAST(NEW.id AS TEXT),
        'db_trigger',
        json_object(
            'default_currency_code', NEW.default_currency_code,
            'display_timezone', NEW.display_timezone,
            'orphan_count_threshold', NEW.orphan_count_threshold,
            'orphan_spending_threshold_bps', NEW.orphan_spending_threshold_bps,
            'onboarding_completed_at_utc', NEW.onboarding_completed_at_utc
        ),
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    );
END;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS trg_audit_settings_update;
DROP TRIGGER IF EXISTS trg_audit_settings_insert;
DROP TRIGGER IF EXISTS trg_audit_monthly_caps_update;
DROP TRIGGER IF EXISTS trg_audit_monthly_caps_insert;
DROP TRIGGER IF EXISTS trg_audit_transactions_soft_delete;
DROP TRIGGER IF EXISTS trg_audit_transactions_insert;
DROP TRIGGER IF EXISTS trg_audit_labels_soft_delete;
DROP TRIGGER IF EXISTS trg_audit_labels_update;
DROP TRIGGER IF EXISTS trg_audit_labels_insert;
DROP TRIGGER IF EXISTS trg_audit_categories_soft_delete;
DROP TRIGGER IF EXISTS trg_audit_categories_update;
DROP TRIGGER IF EXISTS trg_audit_categories_insert;

-- +goose StatementEnd
