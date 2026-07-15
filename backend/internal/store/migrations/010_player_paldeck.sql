CREATE TABLE IF NOT EXISTS player_paldeck (
  player_uid TEXT NOT NULL,
  character_id TEXT NOT NULL,
  capture_count INTEGER,
  unlocked INTEGER,
  PRIMARY KEY(player_uid, character_id)
);

CREATE TABLE IF NOT EXISTS player_paldeck_state (
  player_uid TEXT PRIMARY KEY,
  capture_counts_available INTEGER NOT NULL DEFAULT 0,
  unlock_flags_available INTEGER NOT NULL DEFAULT 0,
  capture_counts_truncated INTEGER NOT NULL DEFAULT 0,
  unlock_flags_truncated INTEGER NOT NULL DEFAULT 0,
  capture_observed_at INTEGER,
  unlock_observed_at INTEGER
);

CREATE INDEX IF NOT EXISTS player_paldeck_character ON player_paldeck(character_id);
