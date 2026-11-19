CREATE TABLE IF NOT EXISTS credit_mail_previews (
  instrument_id VARCHAR(64) NOT NULL,
  household_id VARCHAR(64) NOT NULL,
  generation INTEGER NOT NULL,
  synchronized_at VARCHAR(40) NOT NULL,
  statements_json TEXT NOT NULL,
  choices_json TEXT NOT NULL,
  PRIMARY KEY (household_id, instrument_id)
);
