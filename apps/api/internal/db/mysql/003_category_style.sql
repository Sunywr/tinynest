-- 003: per-category flat icon key and palette colour.
--
-- MySQL DDL commits implicitly, so this file must be re-runnable after a crash
-- that happened between the ALTER and the schema_migrations insert. MySQL 8 has
-- no ADD COLUMN IF NOT EXISTS, so the statement is built from
-- information_schema and executed through a prepared statement.
SET @add_color := (
  SELECT IF(COUNT(*) = 0,
    'ALTER TABLE categories ADD COLUMN color VARCHAR(24) NOT NULL DEFAULT ''''',
    'DO 0')
  FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'categories' AND column_name = 'color'
);
PREPARE add_color_stmt FROM @add_color;
EXECUTE add_color_stmt;
DEALLOCATE PREPARE add_color_stmt;
