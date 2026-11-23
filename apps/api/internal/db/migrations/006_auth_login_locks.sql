CREATE TABLE IF NOT EXISTS auth_login_locks (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    business_day TEXT NOT NULL,
    failures INTEGER NOT NULL,
    rounds INTEGER NOT NULL,
    locked_until INTEGER NOT NULL
);
