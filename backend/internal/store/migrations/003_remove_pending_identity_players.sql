CREATE TABLE IF NOT EXISTS sessions (id INTEGER PRIMARY KEY AUTOINCREMENT, player_uid TEXT NOT NULL, join_at INTEGER NOT NULL, leave_at INTEGER);
CREATE INDEX IF NOT EXISTS sessions_uid ON sessions(player_uid, join_at DESC);
DELETE FROM sessions WHERE player_uid = 'none';
DELETE FROM players WHERE uid = 'none';
