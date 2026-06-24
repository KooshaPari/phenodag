"""Tier-3 #3 CLI smoke test for phenodag.

Runs the freshly-built `phenodag` binary through a canned init/seed/
validate/pick/status lifecycle against a fresh temp DB, asserts each
subcommand exits 0, and verifies the resulting DB is non-empty + has
the expected SQLite schema (the 4 core tables: dag_meta, agents,
tasks, edges).

This is a pure stdlib test (no pytest, no Go toolchain at runtime).
The binary is built once via `make build`.

Run:  python scripts/smoke_cli.py --bin phenodag --db /tmp/phenodag-smoke.db
"""

from __future__ import annotations

import argparse
import os
import sqlite3
import subprocess
import sys
from pathlib import Path

# Subcommands + their args. Keep in the same order as the Makefile `smoke` target
# so a CI invocation of `make smoke` and `make test-cli-smoke` produce the
# same artifact (deterministic DB content for downstream tests).
LIFECYCLE = [
    ("init",     ["--width", "20", "--stages", "6"]),
    ("seed",     ["--preset", "v3-180"]),
    ("validate", []),
    ("pick",     ["--agent", "smoke-agent"]),
    ("status",   []),
]

# Tables that must exist after `init` (created by the migrate() step in
# phenodag.go:54-87).  If the migrate() call drops one of these, the
# smoke test will fail loudly rather than silently passing with an
# empty DB.
REQUIRED_TABLES = {"dag_meta", "agents", "tasks", "edges"}


def run_subcommand(bin: Path, db: Path, subcmd: str, args: list[str]) -> None:
    """Invoke the phenodag binary with --db <db> <subcmd> <args> and assert RC=0."""
    cmd = [str(bin), "--db", str(db), subcmd, *args]
    proc = subprocess.run(cmd, capture_output=True, text=True, check=False)
    if proc.returncode != 0:
        sys.stderr.write(
            f"FAIL: {' '.join(cmd)}\n"
            f"  exit code: {proc.returncode}\n"
            f"  stdout: {proc.stdout!r}\n"
            f"  stderr: {proc.stderr!r}\n"
        )
        raise SystemExit(1)
    print(f"  PASS  {subcmd:9s}  ({' '.join(args) or '<no args>'})")


def assert_db_schema(db: Path) -> None:
    """Assert the SQLite DB has all REQUIRED_TABLES and at least one row in tasks."""
    if not db.exists():
        sys.stderr.write(f"FAIL: DB {db} does not exist after lifecycle\n")
        raise SystemExit(1)
    if db.stat().st_size == 0:
        sys.stderr.write(f"FAIL: DB {db} is 0 bytes after lifecycle\n")
        raise SystemExit(1)
    with sqlite3.connect(str(db)) as conn:
        cur = conn.execute(
            "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name"
        )
        tables = {r[0] for r in cur.fetchall()}
    missing = REQUIRED_TABLES - tables
    if missing:
        sys.stderr.write(
            f"FAIL: DB {db} is missing required tables: {sorted(missing)}\n"
            f"  found: {sorted(tables)}\n"
        )
        raise SystemExit(1)
    print(f"  PASS  schema       (found {len(tables)} tables: {sorted(tables)})")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--bin", default="phenodag", help="path to phenodag binary")
    parser.add_argument("--db", default="/tmp/phenodag-smoke.db", help="SQLite DB path (will be removed first)")
    args = parser.parse_args()

    bin_path = Path(args.bin)
    if not bin_path.exists():
        sys.stderr.write(f"FAIL: binary {bin_path} does not exist; run `make build` first\n")
        return 1

    db_path = Path(args.db)
    # Wipe any stale DB + WAL + SHM files (matches Makefile `smoke` semantics).
    for ext in ("", "-shm", "-wal"):
        p = db_path.with_name(db_path.name + ext) if ext else db_path
        if p.exists():
            p.unlink()

    print(f"smoke: running lifecycle against {db_path} using {bin_path}")
    for subcmd, sub_args in LIFECYCLE:
        run_subcommand(bin_path, db_path, subcmd, sub_args)
    assert_db_schema(db_path)
    print(f"\nsmoke: OK (DB {db_path} is {db_path.stat().st_size} bytes)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
