#!/usr/bin/env python3
"""Perform consistent SQLite online backup using sqlite3 backup API; use on backend B."""
import argparse
from datetime import datetime, timezone
from pathlib import Path
import sqlite3
p=argparse.ArgumentParser()
p.add_argument('--db', required=True)
p.add_argument('--out-dir', required=True)
a=p.parse_args()
db=Path(a.db).expanduser().resolve()
out=Path(a.out_dir).expanduser().resolve()
if not db.is_file(): raise SystemExit(f'Database does not exist: {db}')
if out == db.parent: raise SystemExit('Use a separate backup directory')
out.mkdir(parents=True,exist_ok=True)
dst=out / f'tinynest-{datetime.now(timezone.utc):%Y%m%dT%H%M%SZ}.sqlite3'
source=sqlite3.connect(f'file:{db}?mode=ro',uri=True)
target=sqlite3.connect(str(dst))
try:
    source.backup(target)
    target.commit()
    assert target.execute('PRAGMA quick_check').fetchone()[0] == 'ok'
finally:
    source.close();target.close()
dst.chmod(0o600)
print(f'Validated SQLite backup created: {dst}')
