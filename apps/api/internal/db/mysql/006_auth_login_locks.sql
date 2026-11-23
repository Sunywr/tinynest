CREATE TABLE IF NOT EXISTS auth_login_locks (
    user_id VARCHAR(191) PRIMARY KEY,
    business_day CHAR(10) NOT NULL,
    failures INTEGER NOT NULL,
    rounds INTEGER NOT NULL,
    locked_until BIGINT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
