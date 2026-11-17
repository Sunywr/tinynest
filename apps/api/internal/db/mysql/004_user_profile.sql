-- 004: user self-service profile fields, applied as version 4 on existing databases
ALTER TABLE users
  ADD COLUMN email VARCHAR(254) NULL,
  ADD COLUMN avatar VARCHAR(8) NULL,
  ADD COLUMN birthday CHAR(10) NULL,
  ADD COLUMN payday INT NULL;
