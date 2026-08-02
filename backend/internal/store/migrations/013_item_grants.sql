CREATE TABLE IF NOT EXISTS item_grants (
    request_id TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    actor TEXT NOT NULL,
    player_uid TEXT NOT NULL,
    player_name TEXT NOT NULL,
    item_id TEXT NOT NULL,
    item_name TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK(quantity >= 1 AND quantity <= 9999),
    granted_quantity INTEGER,
    reason TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('queued','succeeded','failed')),
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS item_grants_created_at ON item_grants(created_at DESC);
CREATE INDEX IF NOT EXISTS item_grants_player_uid ON item_grants(player_uid, created_at DESC);
