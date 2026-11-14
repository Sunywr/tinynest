#!/usr/bin/env python3
"""V0.1.1 M6 isolated database drill (NEVER touches a real ledger).

On a throwaway directory this script:
  1. builds the Go API and migrates an EMPTY SQLite (all embedded migrations),
  2. exercises setup + login + one account write through the real HTTP API,
  3. restarts the server against the same file and verifies data survives,
  4. takes an online backup, restores it into a NEW file and verifies
     quick_check + foreign_key_check on the restored copy.

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


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--api-dir", required=True)
    ap.add_argument("--keep", action="store_true")
    a = ap.parse_args()
    api = Path(a.api_dir).resolve()
    work = Path(tempfile.mkdtemp(prefix="tinynest-drill-"))
    print(f"drill workdir: {work}")

    print("[1/6] go build ./cmd/server")
    subprocess.run(["go", "build", "-o", str(work / "tinynest-server"), "./cmd/server"],
                   cwd=api, check=True)

    print("[2/6] start on EMPTY database -> embedded migrations apply")
    (work / "data").mkdir()
    proc, log = start_server(work, "server1.log")

    print("[3/6] setup + login + create account via real HTTP")
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

    print("[4/6] restart server against the SAME file (crash-recovery shape)")
    proc.terminate(); proc.wait(timeout=15); log.close()
    proc, log = start_server(work, "server2.log")
    code, bal = api_call("GET", f"/api/v1/accounts/{account_id}/balance", cookie=cookie)
    assert code == 200 and bal["balance_minor"] == 50000, f"data lost across restart: {bal}"
    print(f"  balance survived restart: {bal['balance_minor']}")

    print("[5/6] online backup -> restore into a NEW file")
    db = work / "data" / "drill.sqlite3"
    backup = work / "backups" / "restored-source.sqlite3"
    backup.parent.mkdir()
    src = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
    dst = sqlite3.connect(str(backup))
    src.backup(dst)
    dst.commit(); dst.close(); src.close()
    restored = work / "backups" / "restored-copy.sqlite3"
    shutil.copy2(backup, restored)
    check(restored)

    print("[6/6] final integrity check on the live database")
    check(db)

    proc.terminate(); proc.wait(timeout=15); log.close()
    print("DRILL PASS: empty-migrate / restart / backup / restore / quick_check / foreign_key_check")
    if not a.keep:
        shutil.rmtree(work, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
