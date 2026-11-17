-- 012: user self-service profile fields (name lives on the bound household_members row)
ALTER TABLE users ADD COLUMN email TEXT;
ALTER TABLE users ADD COLUMN avatar TEXT;
ALTER TABLE users ADD COLUMN birthday TEXT;
ALTER TABLE users ADD COLUMN payday INTEGER;
