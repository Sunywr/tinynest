CREATE TABLE IF NOT EXISTS credit_mailboxes (
  id VARCHAR(64) PRIMARY KEY,
  household_id VARCHAR(64) NOT NULL REFERENCES households(id),
  host VARCHAR(253) NOT NULL,
  username VARCHAR(320) NOT NULL,
  folder VARCHAR(255) NOT NULL,
  password_cipher TEXT NOT NULL,
  enabled INTEGER NOT NULL,
  version INTEGER NOT NULL,
  tested_at VARCHAR(40) NOT NULL
);

CREATE TABLE IF NOT EXISTS credit_mail_bindings (
  instrument_id VARCHAR(64) PRIMARY KEY,
  household_id VARCHAR(64) NOT NULL REFERENCES households(id),
  mailbox_id VARCHAR(64) NOT NULL REFERENCES credit_mailboxes(id),
  category_id VARCHAR(64) NOT NULL,
  subject_keyword VARCHAR(255) NOT NULL,
  card_tail VARCHAR(4) NOT NULL
);
