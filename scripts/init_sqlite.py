#!/usr/bin/env python3
"""Idempotent Phase-0 SQLite bootstrap, NO financial data schema."""
import argparse
import os
from pathlib import Path
import sqlite3

parser = argparse.ArgumentParser()
parser.add_argument('--db', required=True, help='Absolute/relative SQLite database path on backend host')
a = parser.parse_args()
path = Path(a.db).expanduser().resolve()
path.parent.mkdir(parents=True, exist_ok=True)
if path.is_symlink():
    raise SystemExit('Refusing a symlinked database file')
conn = sqlite3.connect(str(path), timeout=10)
try:
    conn.execute('PRAGMA journal_mode=WAL')
    conn.execute('PRAGMA foreign_keys=ON')
    conn.execute("CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, description TEXT NOT NULL, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)")
    conn.commit()
    state = conn.execute('PRAGMA quick_check').fetchone()
    assert state and state[0] == 'ok', f'quick_check failed: {state}'
finally:
    conn.close()
if os.name == 'posix':
    os.chmod(path, 0o600)
print(f'Phase 0 SQLite metadata ready: {path} (no business tables)')
