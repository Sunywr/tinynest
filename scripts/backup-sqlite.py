#!/usr/bin/env python3
"""Consistent SQLite online backup with VERIFIED artifact (backend B only).

Uses sqlite3's online backup API so committed WAL content is included. The
source is opened read-only and never modified. The produced file is then
verified against the source:

  * PRAGMA quick_check == 'ok' and foreign_key_check == 0
  * identical schema_migrations versions
  * identical business snapshot (accounts, transactions, journal entries /
    lines, debit/credit totals, opening balances, credit contracts,
    installment contracts, debt contracts, investment assets)
  * balanced journal (sum(debit) == sum(credit))
  * SHA-256 printed for off-site integrity verification
  * file mode 0600

No real ledger is touched unless the operator explicitly passes --db.
"""
import argparse
from datetime import datetime, timezone
import hashlib
import os
from pathlib import Path
import sqlite3
import sys
import tempfile
from contextlib import closing

# Every value here is part of the "business snapshot" an operator must be able
# to reconcile after a restore. Keep in sync with scripts/db_drill.py.
SNAPSHOT_QUERIES = {
    "migrations": "SELECT version FROM schema_migrations ORDER BY version",
    "accounts": "SELECT COUNT(*) FROM accounts",
    "transactions": "SELECT COUNT(*) FROM transactions",
    "journal_entries": "SELECT COUNT(*) FROM journal_entries",
    "journal_lines": "SELECT COUNT(*) FROM journal_lines",
    "debit_minor": "SELECT COALESCE(SUM(debit_minor),0) FROM journal_lines",
    "credit_minor": "SELECT COALESCE(SUM(credit_minor),0) FROM journal_lines",
    "opening_minor": "SELECT COALESCE(SUM(amount_minor),0) FROM account_opening_balances",
    "credit_instruments": "SELECT COUNT(*) FROM credit_instruments",
    "installment_contracts": "SELECT COUNT(*) FROM installment_contracts",
    "debt_contracts": "SELECT COUNT(*) FROM debt_contracts",
    "investment_assets": "SELECT COUNT(*) FROM investment_assets",
}


def snapshot(conn: sqlite3.Connection) -> dict:
    out = {}
    for key, sql in SNAPSHOT_QUERIES.items():
        if key == "migrations":
            out[key] = [r[0] for r in conn.execute(sql)]
        else:
            out[key] = conn.execute(sql).fetchone()[0]
    return out


def integrity(conn: sqlite3.Connection, label: str) -> None:
    quick = conn.execute("PRAGMA quick_check").fetchone()[0]
    if quick != "ok":
        raise RuntimeError(f"{label}: quick_check failed: {quick}")
    if conn.execute("PRAGMA foreign_key_check").fetchone() is not None:
        raise RuntimeError(f"{label}: foreign_key_check failed")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", required=True, help="source SQLite database (read-only)")
    ap.add_argument("--out-dir", required=True, help="separate backup directory")
    a = ap.parse_args()

    db = Path(a.db).expanduser().resolve()
    out = Path(a.out_dir).expanduser().resolve()
    if not db.is_file():
        raise SystemExit(f"Database does not exist: {db}")
    if out == db.parent:
        raise SystemExit("Use a separate backup directory")
    out.mkdir(parents=True, exist_ok=True)

    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    fd, temp_name = tempfile.mkstemp(prefix=".tinynest-", suffix=".tmp", dir=out)
    os.close(fd)
    temp = Path(temp_name)
    dst = out / f"tinynest-{stamp}-{temp.stem.removeprefix('.tinynest-')}.sqlite3"
    try:
        # Pin the source snapshot before copying so concurrent commits cannot
        # make a consistent backup fail reconciliation.
        with closing(sqlite3.connect(f"{db.as_uri()}?mode=ro", uri=True)) as source:
            source.execute("BEGIN")
            src_snap = snapshot(source)
            with closing(sqlite3.connect(temp)) as target:
                source.backup(target)
                integrity(target, "backup")
                bak_snap = snapshot(target)
                if src_snap != bak_snap:
                    raise RuntimeError("Backup business snapshot differs from source")
                if bak_snap["debit_minor"] != bak_snap["credit_minor"]:
                    raise RuntimeError("Backup journal lines are not balanced")
                if not bak_snap["migrations"]:
                    raise RuntimeError("Backup is missing migration versions")

        with temp.open("rb") as artifact:
            digest = hashlib.file_digest(artifact, "sha256").hexdigest()
        temp.replace(dst)
    finally:
        temp.unlink(missing_ok=True)

    print(f"Validated SQLite backup created: {dst}")
    print(f"  sha256={digest}")
    print(f"  quick_check=ok foreign_key_check=0 migrations={bak_snap['migrations']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
