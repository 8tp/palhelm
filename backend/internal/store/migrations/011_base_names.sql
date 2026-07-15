-- 011: player-chosen base camp names, decoded from BaseCampSaveData.RawData.
-- NULL means the base was never renamed (or the row predates name decoding);
-- the API serves NULL, never "" or a synthetic label. Rows written before this
-- migration stay NULL until the next save parse repopulates the table.
--
-- Rebuild-table pattern (like 004) rather than a bare ALTER ADD COLUMN so an
-- interrupted-migration replay stays idempotent: replaying this file on a
-- database that already carries the column succeeds instead of failing on a
-- duplicate column.
CREATE TABLE IF NOT EXISTS bases (id TEXT PRIMARY KEY, guild_id TEXT, x REAL, y REAL, level INTEGER NOT NULL DEFAULT 0);

DROP TABLE IF EXISTS bases_v011;
CREATE TABLE bases_v011 (
  id TEXT PRIMARY KEY,
  guild_id TEXT,
  name TEXT,
  x REAL,
  y REAL,
  level INTEGER NOT NULL DEFAULT 0
);
INSERT INTO bases_v011(id,guild_id,x,y,level)
SELECT id,guild_id,x,y,level FROM bases;
DROP TABLE bases;
ALTER TABLE bases_v011 RENAME TO bases;
