-- 003: per-category flat icon key and palette colour.
-- SQLite runs a migration and its ledger row in one transaction, so a plain
-- ADD COLUMN is atomic and needs no separate existence guard.
ALTER TABLE categories ADD COLUMN color TEXT NOT NULL DEFAULT '';
