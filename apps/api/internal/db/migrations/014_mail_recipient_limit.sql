CREATE TABLE IF NOT EXISTS mail_recipient_daily_usage (
 day VARCHAR(10) NOT NULL,
 email VARCHAR(254) NOT NULL,
 total INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(day,email)
);
