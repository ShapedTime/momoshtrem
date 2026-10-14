-- Key/value application settings (quality profile, AI settings, classifier
-- defaults). Each value is a JSON document owned by the feature that reads it.
CREATE TABLE IF NOT EXISTS app_settings (
    key        TEXT PRIMARY KEY,
    value      JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
