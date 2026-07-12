CREATE TABLE IF NOT EXISTS api_keys (
  id           TEXT PRIMARY KEY,          -- 8-char public key id
  hash         BLOB NOT NULL UNIQUE,      -- 32-byte SHA-256 of the full plaintext key
  label        TEXT NOT NULL,
  created_at   INTEGER NOT NULL,          -- unix seconds, like every other table
  last_used_at INTEGER,
  revoked_at   INTEGER
);
