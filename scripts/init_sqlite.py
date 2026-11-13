#!/usr/bin/env python3
"""Phase-0 legacy SQLite metadata bootstrap (NOT the V0.1 schema installer).

Creates only the schema_migrations ledger so older Phase-0 deployments keep a
compatible on-disk shape. All V0.1 business tables are created by the Go API's
embedded, versioned migrations on startup; this script never creates them.
"""
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
print(f'Phase-0 metadata ready: {path} (schema_migrations only; V0.1 business tables are applied by the Go API migrations on startup)')
