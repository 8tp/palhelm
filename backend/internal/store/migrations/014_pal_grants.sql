CREATE TABLE IF NOT EXISTS pal_grants (
    request_id TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    actor TEXT NOT NULL,
    player_uid TEXT NOT NULL,
    player_name TEXT NOT NULL,
    character_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    spec_json TEXT NOT NULL,
    result_json TEXT,
    reason TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('queued','succeeded','failed')),
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS pal_grants_created_at ON pal_grants(created_at DESC);
CREATE INDEX IF NOT EXISTS pal_grants_player_uid ON pal_grants(player_uid, created_at DESC);
