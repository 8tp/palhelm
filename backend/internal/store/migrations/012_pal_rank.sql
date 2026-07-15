-- Pal Condenser rank (1 = never condensed, up to 5 = 4 stars). Nullable so a pal
-- parsed before this column existed reads back as NULL (unavailable), never a
-- misleading 0. Soul-enhancement Rank_HP/Rank_Attack/Rank_Defence are out of scope.
ALTER TABLE pals ADD COLUMN rank INTEGER;
