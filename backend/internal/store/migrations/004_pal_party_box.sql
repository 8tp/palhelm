CREATE TABLE IF NOT EXISTS pals (
  instance_id TEXT PRIMARY KEY,
  owner_uid TEXT,
  character_id TEXT,
  display_name TEXT NOT NULL DEFAULT '',
  level INTEGER,
  is_alpha INTEGER,
  is_lucky INTEGER,
  raw_json TEXT NOT NULL DEFAULT '{}'
);

DROP TABLE IF EXISTS pals_v004;
CREATE TABLE pals_v004 (
  instance_id TEXT PRIMARY KEY,
  owner_uid TEXT,
  character_id TEXT,
  display_name TEXT NOT NULL DEFAULT '',
  level INTEGER,
  is_alpha INTEGER,
  is_lucky INTEGER,
  raw_json TEXT NOT NULL DEFAULT '{}',
  in_party INTEGER NOT NULL DEFAULT 0,
  party_slot INTEGER,
  box_page INTEGER,
  box_slot INTEGER
);
INSERT INTO pals_v004(instance_id,owner_uid,character_id,display_name,level,is_alpha,is_lucky,raw_json)
SELECT instance_id,owner_uid,character_id,display_name,level,is_alpha,is_lucky,raw_json FROM pals;
DROP TABLE pals;
ALTER TABLE pals_v004 RENAME TO pals;
