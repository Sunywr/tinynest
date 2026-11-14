CREATE TABLE IF NOT EXISTS pending_registrations (
  username VARCHAR(100) PRIMARY KEY,
  password_hash VARCHAR(191) NOT NULL,
  created_at VARCHAR(32) NOT NULL
);
CREATE TABLE IF NOT EXISTS household_invites (
  token_hash VARCHAR(64) PRIMARY KEY,
  household_id VARCHAR(191) NOT NULL,
  member_id VARCHAR(191) NOT NULL,
  target_username VARCHAR(100),
  expires_at VARCHAR(32) NOT NULL,
  applicant VARCHAR(100),
  decided_at VARCHAR(32)
);
CREATE TABLE IF NOT EXISTS household_user_members (
  user_id VARCHAR(191) PRIMARY KEY,
  member_id VARCHAR(191) NOT NULL UNIQUE
);
