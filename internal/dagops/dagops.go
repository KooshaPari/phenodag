// Package dagops provides the shared "open a SQLite DB, run migrate,
// and apply the advisory flock while fn runs" pattern that was repeated
// ~30 times across phenodag_extras.go as a verbatim dagctl port.
//
// Phase-4b (issue #5) hoists this pattern to one place so both the
// pre-merge phenodag.go core and the post-merge cmd*Port funcs use the
// same code path. See ADR-dag-superset-merge.md and
// ADR-dedup-baseline.md.
package dagops

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open opens the SQLite DB at path with the same pragmas phenodag uses
// (WAL, busy_timeout, foreign_keys). SetMaxOpenConns(1) is preserved so
// the existing single-writer claim semantics are unchanged.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Migrate applies the standard phenodag schema. Migrations are
// idempotent (CREATE IF NOT EXISTS, ALTER ADD COLUMN with safe default).
func Migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS dag_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS agents (id TEXT PRIMARY KEY, status TEXT NOT NULL DEFAULT 'active', last_seen TEXT NOT NULL, last_heartbeat TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY, stage INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL DEFAULT 0,
			description TEXT NOT NULL, repo TEXT NOT NULL DEFAULT '', subproject TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT '', lane TEXT NOT NULL DEFAULT '', branch TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT 'task', priority INTEGER NOT NULL DEFAULT 5,
			semantic_hash TEXT NOT NULL DEFAULT '', side_dag TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'ready', assigned_agent TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS edges (from_task TEXT NOT NULL, to_task TEXT NOT NULL, PRIMARY KEY (from_task, to_task))`,
		`CREATE TABLE IF NOT EXISTS claims (
			agent TEXT NOT NULL, task_id TEXT NOT NULL, repo TEXT NOT NULL, branch TEXT NOT NULL DEFAULT '',
			worktree TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'active', claimed_at TEXT NOT NULL,
			PRIMARY KEY (repo, branch, worktree))`,
		`CREATE TABLE IF NOT EXISTS duplicate_groups (id INTEGER PRIMARY KEY AUTOINCREMENT, root_id TEXT NOT NULL, similarity REAL NOT NULL, resolution TEXT NOT NULL DEFAULT 'unresolved', tasks TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS repos (
			name TEXT PRIMARY KEY, is_local INTEGER NOT NULL DEFAULT 0, is_git INTEGER NOT NULL DEFAULT 1,
			is_mangled INTEGER NOT NULL DEFAULT 0, is_claimed INTEGER NOT NULL DEFAULT 0,
			current_branch TEXT NOT NULL DEFAULT '', branch_count INTEGER NOT NULL DEFAULT 0,
			worktree_count INTEGER NOT NULL DEFAULT 0, has_uncommitted INTEGER NOT NULL DEFAULT 0,
			stashes INTEGER NOT NULL DEFAULT 0, open_prs INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS side_dags (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	for _, s := range []string{
		`ALTER TABLE agents ADD COLUMN last_heartbeat TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE repos ADD COLUMN is_mangled INTEGER NOT NULL DEFAULT 0`,
	} {
		_, _ = db.Exec(s)
	}
	for _, s := range []string{
		`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_stage ON tasks(stage)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_assigned ON tasks(assigned_agent)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_task)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_to ON edges(to_task)`,
		`CREATE INDEX IF NOT EXISTS idx_claims_agent ON claims(agent)`,
	} {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("index: %w", err)
		}
	}
	return nil
}

// withLock is the same advisory-lock wrapper phenodag.go uses. In the
// merged codebase it is replaced by remoteclaim.WithLock which provides
// real POSIX flock semantics; for now the no-op stub is preserved so
// the existing single-writer SQLite path is unchanged.
func withLock(path string, fn func() error) error {
	_ = path
	return fn()
}

// OpenLocked opens the DB at path, runs Migrate, and calls fn with the
// open *sql.DB. The function is wrapped in the shared advisory-lock
// path (a no-op in this repo since SQLite WAL busy_timeout is the
// real serialization mechanism; see phenodag.go:withLock).
func OpenLocked(path string, fn func(*sql.DB) error) error {
	return withLock(path, func() error {
		db, err := Open(path)
		if err != nil {
			return err
		}
		defer db.Close()
		if err := Migrate(db); err != nil {
			return err
		}
		return fn(db)
	})
}

// OpenRO opens the DB at path without running Migrate. Used by read-only
// commands (status, validate, export, viz) that should not require
// write access to the schema.
func OpenRO(path string, fn func(*sql.DB) error) error {
	return withLock(path, func() error {
		db, err := Open(path)
		if err != nil {
			return err
		}
		defer db.Close()
		return fn(db)
	})
}
