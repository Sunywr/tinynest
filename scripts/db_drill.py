#!/usr/bin/env python3
"""V0.1.1 M6 isolated database drill (NEVER touches a real ledger).

On a throwaway directory this script:
  1. builds the Go API and migrates an EMPTY SQLite (all embedded migrations),
  2. exercises setup + login + one account write through the real HTTP API,
  3. restarts the server against the same file and verifies data survives,
  4. restarts a THIRD time to prove repeat migrations are a no-op,
  5. verifies the journal is balanced (sum(debit) == sum(credit)),
  6. takes an online backup and restores it into a NEW file,
  7. compares full business snapshots of live and restored databases,
  8. verifies quick_check + foreign_key_check on both copies.

Usage: python3 scripts/db_drill.py --api-dir apps/api [--keep]
Requires: go on PATH. Listens on 127.0.0.1:18788 (never a production port).
"""
import argparse
import http.cookiejar
import json
import os
from pathlib import Path
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import time
import urllib.request

HEALTH = "http://127.0.0.1:18788/api/v1/healthz"
SETUP_TOKEN = "db-drill-token-not-a-real-secret"


def api_call(method: str, path: str, body=None, cookie=None, csrf=None, setup=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request("http://127.0.0.1:18788" + path, data=data, method=method)
    if data:
        req.add_header("Content-Type", "application/json")
    if cookie:
        req.add_header("Cookie", cookie)
    if csrf:
        req.add_header("X-CSRF-Token", csrf)
    if setup:
        req.add_header("X-Setup-Token", setup)
    with urllib.request.urlopen(req, timeout=10) as res:
        return res.status, json.loads(res.read().decode() or "{}")


def wait_health(proc, deadline_s: int = 60) -> None:
    end = time.time() + deadline_s
    while time.time() < end:
        if proc.poll() is not None:
            raise SystemExit("server exited during startup; see server log")
        try:
            if urllib.request.urlopen(HEALTH, timeout=2).status == 200:
                return
        except Exception:
            time.sleep(0.3)
    raise SystemExit("server did not become healthy in time")


def start_server(workdir: Path, logname: str):
    log = open(workdir / logname, "w")
    proc = subprocess.Popen(
        [str(workdir / "tinynest-server")],
        env={
            **os.environ,  # full environment: Winsock needs SystemRoot etc.
            "TINYNEST_LISTEN_ADDR": "127.0.0.1:18788",
            "TINYNEST_DATA_DIR": str(workdir / "data"),
            "TINYNEST_DB_PATH": str(workdir / "data" / "drill.sqlite3"),
            "TINYNEST_SETUP_TOKEN": SETUP_TOKEN,
            "TINYNEST_COOKIE_SECURE": "false",
            "TINYNEST_TRUSTED_PROXIES": "",
        },
        stdout=log, stderr=log,
    )
    wait_health(proc)
    return proc, log


def check(db_path: Path) -> None:
    conn = sqlite3.connect(str(db_path))
    try:
        quick = conn.execute("PRAGMA quick_check").fetchone()[0]
        fk_rows = conn.execute("PRAGMA foreign_key_check").fetchall()
        versions = [r[0] for r in conn.execute("SELECT version FROM schema_migrations ORDER BY version")]
    finally:
        conn.close()
    assert quick == "ok", f"quick_check failed: {quick}"
    assert not fk_rows, f"foreign_key_check violations: {fk_rows[:5]}"
    print(f"  quick_check=ok foreign_key_check=0 migrations={versions}")


def business_snapshot(db_path: Path) -> dict:
    """Business totals rebuilt from the journal, used to prove a restore is identical."""
    conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    try:
        return {
            "migrations": [r[0] for r in conn.execute(
                "SELECT version FROM schema_migrations ORDER BY version")],
            "accounts": conn.execute("SELECT COUNT(*) FROM accounts").fetchone()[0],
            "journal_entries": conn.execute("SELECT COUNT(*) FROM journal_entries").fetchone()[0],
            "journal_lines": conn.execute("SELECT COUNT(*) FROM journal_lines").fetchone()[0],
            "debit_minor": conn.execute(
                "SELECT COALESCE(SUM(debit_minor),0) FROM journal_lines").fetchone()[0],
            "credit_minor": conn.execute(
                "SELECT COALESCE(SUM(credit_minor),0) FROM journal_lines").fetchone()[0],
            "opening_minor": conn.execute(
                "SELECT COALESCE(SUM(amount_minor),0) FROM account_opening_balances").fetchone()[0],
        }
    finally:
        conn.close()


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--api-dir", required=True)
    ap.add_argument("--keep", action="store_true")
    a = ap.parse_args()
    api = Path(a.api_dir).resolve()
    work = Path(tempfile.mkdtemp(prefix="tinynest-drill-"))
    print(f"drill workdir: {work}")

    print("[1/8] go build ./cmd/server")
    subprocess.run(["go", "build", "-o", str(work / "tinynest-server"), "./cmd/server"],
                   cwd=api, check=True)

    print("[2/8] start on EMPTY database -> embedded migrations apply")
    (work / "data").mkdir()
    proc, log = start_server(work, "server1.log")

    print("[3/8] setup + login + create account via real HTTP")
    code, _ = api_call("POST", "/api/v1/setup",
                   {"household_name": "演练小家", "username": "admin", "password": "correct-horse-battery"},
                   setup=SETUP_TOKEN)
    assert code == 201, "setup failed"
    # Keep the session cookie exactly like a browser would.
    jar = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    req = urllib.request.Request("http://127.0.0.1:18788/api/v1/auth/session",
                                 data=json.dumps({"username": "admin", "password": "correct-horse-battery"}).encode(),
                                 method="POST")
    req.add_header("Content-Type", "application/json")
    with opener.open(req, timeout=10) as res:
        csrf = json.loads(res.read().decode())["csrf_token"]
    raw = [c.value for c in jar if c.name == "tn_session"][0]
    cookie = "tn_session=" + raw
    code, acct = api_call("POST", "/api/v1/accounts",
                      {"kind": "cash", "name": "演练卡", "opening_balance_minor": 50000},
                      cookie=cookie, csrf=csrf)
    assert code == 201, f"account create failed: {acct}"
    account_id = acct["id"]

    print("[4/8] restart server against the SAME file (crash-recovery shape)")
    proc.terminate(); proc.wait(timeout=15); log.close()
    proc, log = start_server(work, "server2.log")
    code, bal = api_call("GET", f"/api/v1/accounts/{account_id}/balance", cookie=cookie)
    assert code == 200 and bal["balance_minor"] == 50000, f"data lost across restart: {bal}"
    print(f"  balance survived restart: {bal['balance_minor']}")

    db = work / "data" / "drill.sqlite3"
    print("[5/8] start a THIRD time on the existing DB (repeat migrations are a no-op)")
    snap_before = business_snapshot(db)
    proc.terminate(); proc.wait(timeout=15); log.close()
    proc, log = start_server(work, "server3.log")
    snap_after = business_snapshot(db)
    assert snap_before == snap_after, f"repeat migration changed the database: {snap_before} -> {snap_after}"
    assert snap_after["debit_minor"] == snap_after["credit_minor"], "journal lines are not balanced"
    print(f"  repeat-migration snapshot unchanged: {snap_after}")

    print("[6/8] online backup via the REAL operator script -> restore into a NEW file")
    # Step 6 exercises scripts/backup-sqlite.py itself (quick_check,
    # foreign_key_check, migration versions, business snapshot and SHA-256),
    # rather than re-implementing backup() inline. An operator's restore is
    # only as trustworthy as the script that produced the artifact.
    backup_dir = work / "backups"
    backup_dir.mkdir()
    script = Path(__file__).with_name("backup-sqlite.py")
    backup_result = subprocess.run(
        [sys.executable, str(script), "--db", str(db), "--out-dir", str(backup_dir)],
        check=True, capture_output=True, text=True,
    )
    for line in backup_result.stdout.strip().splitlines():
        print("  " + line)
    produced = sorted(backup_dir.glob("tinynest-*.sqlite3"))
    assert len(produced) == 1, f"expected exactly one backup artifact, got {produced}"
    backup = produced[0]
    check(backup)
    restored = work / "backups" / "restored-copy.sqlite3"
    shutil.copy2(backup, restored)
    check(restored)

    print("[7/8] compare business summaries of live and restored databases")
    live = business_snapshot(db)
    copy = business_snapshot(restored)
    assert live == copy, f"restored DB differs from live: {live} vs {copy}"
    assert copy["debit_minor"] == copy["credit_minor"], "restored journal lines are not balanced"
    print(f"  identical business snapshot: {copy}")

    print("[8/8] final integrity check on the live database")
    check(db)

    proc.terminate(); proc.wait(timeout=15); log.close()
    print("DRILL PASS: empty-migrate / restart / repeat-migrate / balanced-journal / backup / restore / snapshot-match / quick_check / foreign_key_check")
    if not a.keep:
        shutil.rmtree(work, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
