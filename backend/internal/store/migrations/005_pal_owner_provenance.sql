ALTER TABLE pals ADD COLUMN owner_source TEXT NOT NULL DEFAULT 'unresolved'
  CHECK(owner_source IN ('save','personal_container','last_observed','unresolved'));

-- Older rows predate provenance recording. Personal placement is still
-- authoritative; any other joined owner is conservatively historical rather
-- than being mislabeled as a current raw-save assertion.
UPDATE pals
SET owner_source = CASE
  WHEN owner_uid IS NULL OR owner_uid = '' THEN 'unresolved'
  WHEN in_party = 1 OR box_page IS NOT NULL THEN 'personal_container'
  ELSE 'last_observed'
END;
