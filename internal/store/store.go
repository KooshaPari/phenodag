// Package store — SQLite + flock storage for phenodag.
//
// One management file (the .db) with WAL journal. POSIX flock on the
// file path provides cross-process mutex; SQLite BEGIN IMMEDIATE
// provides intra-process serialization. Reads are lock-free (WAL
// allows concurrent readers).
//
// Schema is created idempotently on Open; migrations are applied
// via PRAGMA user_version.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

// schemaVersion is bumped when the schema changes.
// Migrations are applied in migrate() based on the delta.
const schemaVersion = 1

// schema is the canonical table DDL. v1.
const schema = `
CREATE TABLE IF NOT EXISTS dag_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
  id              TEXT PRIMARY KEY,
  stage           INTEGER NOT NULL DEFAULT 0,
  slot            INTEGER NOT NULL DEFAULT 0,
  description     TEXT NOT NULL DEFAULT '',
  repo            TEXT NOT NULL DEFAULT '',
  subproject      TEXT NOT NULL DEFAULT '',
  category        TEXT NOT NULL DEFAULT '',
  lane            TEXT NOT NULL DEFAULT '',
  branch          TEXT NOT NULL DEFAULT '',
  worktree        TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'pending',  -- pending|ready|in_progress|done|failed|blocked
  kind            TEXT NOT NULL DEFAULT 'task',
  priority        INTEGER NOT NULL DEFAULT 5,
  semantic_hash   TEXT NOT NULL DEFAULT '',
  side_dag        TEXT NOT NULL DEFAULT '',
  assigned_agent  TEXT NOT NULL DEFAULT '',
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tasks_status  ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_stage   ON tasks(stage);
CREATE INDEX IF NOT EXISTS idx_tasks_repo    ON tasks(repo);
CREATE INDEX IF NOT EXISTS idx_tasks_sidedag ON tasks(side_dag);

CREATE TABLE IF NOT EXISTS edges (
  from_task TEXT NOT NULL,
  to_task   TEXT NOT NULL,
  PRIMARY KEY (from_task, to_task)
);
CREATE INDEX IF NOT EXISTS idx_edges_to ON edges(to_task);

CREATE TABLE IF NOT EXISTS agents (
  id              TEXT PRIMARY KEY,
  status          TEXT NOT NULL DEFAULT 'active',
  last_heartbeat  TEXT NOT NULL,
  created_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS claims (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  agent           TEXT NOT NULL,
  task_id         TEXT NOT NULL DEFAULT '',
  resource        TEXT NOT NULL,
  resource_type   TEXT NOT NULL DEFAULT 'repo',  -- repo|branch|worktree|task
  branch          TEXT NOT NULL DEFAULT '',
  worktree        TEXT NOT NULL DEFAULT '',
  claimed_at      TEXT NOT NULL,
  last_heartbeat  TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'active',  -- active|released|reaped
  UNIQUE(resource, resource_type, branch, worktree)
);
CREATE INDEX IF NOT EXISTS idx_claims_agent ON claims(agent);
CREATE INDEX IF NOT EXISTS idx_claims_status ON claims(status);

CREATE TABLE IF NOT EXISTS duplicate_groups (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  root_id     TEXT NOT NULL,
  similarity  REAL NOT NULL,
  resolution  TEXT NOT NULL DEFAULT 'unresolved',  -- unresolved|merged|skipped
  tasks       TEXT NOT NULL DEFAULT '',  -- JSON array of task IDs
  detected_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_dupe_root ON duplicate_groups(root_id);

CREATE TABLE IF NOT EXISTS side_dags (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS repos (
  path            TEXT PRIMARY KEY,
  name            TEXT NOT NULL DEFAULT '',
  is_local        INTEGER NOT NULL DEFAULT 0,
  is_git          INTEGER NOT NULL DEFAULT 0,
  is_mangled      INTEGER NOT NULL DEFAULT 0,
  branch_count    INTEGER NOT NULL DEFAULT 0,
  worktree_count  INTEGER NOT NULL DEFAULT 0,
  stashes         INTEGER NOT NULL DEFAULT 0,
  has_uncommitted INTEGER NOT NULL DEFAULT 0,
  open_prs        INTEGER NOT NULL DEFAULT 0,
  current_branch  TEXT NOT NULL DEFAULT '',
  is_claimed      INTEGER NOT NULL DEFAULT 0,
  last_scanned    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_repos_local ON repos(is_local);
CREATE INDEX IF NOT EXISTS idx_repos_claimed ON repos(is_claimed);

CREATE TABLE IF NOT EXISTS branches (
  repo_path TEXT NOT NULL,
  name      TEXT NOT NULL,
  is_local  INTEGER NOT NULL DEFAULT 1,
  is_worktree INTEGER NOT NULL DEFAULT 0,
  last_commit TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (repo_path, name)
);

CREATE TABLE IF NOT EXISTS worktrees (
  repo_path TEXT NOT NULL,
  branch    TEXT NOT NULL,
  path      TEXT NOT NULL,
  is_mangled INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (repo_path, path)
);
`

// Store is the SQLite + flock handle.
type Store struct {
	db     *sql.DB
	dbPath string
	lockFD *os.File
}

// Open creates or opens the database at dbPath, applies migrations,
// and acquires the flock. Returns a Store ready for use.
func Open(dbPath string) (*Store, error) {
	// Make sure the directory exists.
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	// Acquire the flock via a sidecar .lock file (POSIX flock is per-FD,
	// so we need a stable FD that other processes can find).
	lockPath := dbPath + ".lock"
	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("flock busy (another process holds the DAG): %w", err)
	}
	// Truncate the lock file to a single byte (a "lock held" marker).
	_ = f.Truncate(1)
	_, _ = f.WriteAt([]byte("1"), 0)

	// Open SQLite with WAL.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // serialize writes; reads are still concurrent via WAL

	// Apply schema.
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		f.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	// Set user_version to schemaVersion.
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		db.Close()
		f.Close()
		return nil, fmt.Errorf("set user_version: %w", err)
	}

	s := &Store{db: db, dbPath: dbPath, lockFD: f}
	return s, nil
}

// Close releases the flock and closes the DB.
func (s *Store) Close() error {
	if s.db != nil {
		_ = s.db.Close()
	}
	if s.lockFD != nil {
		_ = syscall.Flock(int(s.lockFD.Fd()), syscall.LOCK_UN)
		_ = s.lockFD.Close()
	}
	return nil
}

// DB returns the underlying *sql.DB (for ad-hoc queries).
// Use with care — write transactions should be wrapped in WithTx.
func (s *Store) DB() *sql.DB { return s.db }

// WithTx runs fn inside BEGIN IMMEDIATE ... COMMIT.
// Rolls back on panic or error.
func (s *Store) WithTx(fn func(tx *sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// NowUTC returns the current time in RFC3339 UTC.
func NowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// SetMeta upserts a key in dag_meta.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO dag_meta(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// GetMeta fetches a key from dag_meta. Returns "" if not set.
func (s *Store) GetMeta(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM dag_meta WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// AllMeta returns all dag_meta rows as a map.
func (s *Store) AllMeta() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM dag_meta`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			out[k] = v
		}
	}
	return out, nil
}

// ErrAlreadyClaimed is returned by Claim when the resource is already taken.
type ErrAlreadyClaimed struct{ Resource, ResourceType string }

func (e ErrAlreadyClaimed) Error() string {
	return fmt.Sprintf("%s %q is already claimed", e.ResourceType, e.Resource)
}

// SeedTask is a single task to be inserted by `phenodag seed`.
type SeedTask struct {
	ID           string
	Stage        int
	Slot         int
	Description  string
	Repo         string
	Subproject   string
	Category     string
	Lane         string
	Branch       string
	Worktree     string
	Kind         string
	Priority     int
	SemanticHash string
	SideDAG      string
	Status       string
}

// InsertSeedTask inserts a single task. Used by seed.
func (s *Store) InsertSeedTask(t SeedTask) error {
	status := t.Status
	if status == "" {
		status = "ready"
	}
	now := NowUTC()
	_, err := s.db.Exec(`INSERT OR REPLACE INTO tasks
		(id, stage, slot, description, repo, subproject, category, lane, branch, worktree,
		 kind, priority, semantic_hash, side_dag, status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Stage, t.Slot, t.Description, t.Repo, t.Subproject, t.Category, t.Lane,
		t.Branch, t.Worktree, t.Kind, t.Priority, t.SemanticHash, t.SideDAG, status, now, now)
	return err
}

// InsertEdge inserts an edge from → to.
func (s *Store) InsertEdge(from, to string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO edges(from_task, to_task) VALUES (?,?)`, from, to)
	return err
}

// CountTasks returns the total task count, or count of status=match if non-empty.
func (s *Store) CountTasks(status string) (int, error) {
	var n int
	var err error
	if status == "" {
		err = s.db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&n)
	} else {
		err = s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE status=?`, status).Scan(&n)
	}
	return n, err
}

// CountTasksByStatus returns a map status → count.
func (s *Store) CountTasksByStatus() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM tasks GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		_ = rows.Scan(&k, &n)
		out[k] = n
	}
	return out, nil
}

// CountTasksByStage returns a map stage → count (core tasks only).
func (s *Store) CountTasksByStage() (map[int]int, error) {
	rows, err := s.db.Query(`SELECT stage, COUNT(*) FROM tasks
		WHERE side_dag IS NULL OR side_dag=''
		GROUP BY stage`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var k, n int
		_ = rows.Scan(&k, &n)
		out[k] = n
	}
	return out, nil
}

// PickReadyTask atomically claims the highest-priority ready task for an agent.
// Returns the task ID, subproject, description, kind. Empty string = no task available.
func (s *Store) PickReadyTask(agentID string) (string, string, string, string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", "", "", "", err
	}
	defer tx.Rollback()
	now := NowUTC()
	// Try core tasks first (side_dag IS NULL OR '')
	var id, desc, sub, kind string
	err = tx.QueryRow(`SELECT id, description, COALESCE(subproject,''), COALESCE(kind,'')
		FROM tasks
		WHERE status='ready' AND (side_dag IS NULL OR side_dag='')
		ORDER BY priority DESC, stage ASC, slot ASC
		LIMIT 1`).Scan(&id, &desc, &sub, &kind)
	if err == sql.ErrNoRows {
		// Try side-DAG tasks
		err = tx.QueryRow(`SELECT id, description, COALESCE(subproject,''), COALESCE(kind,'')
			FROM tasks
			WHERE status='ready' AND side_dag IS NOT NULL AND side_dag != ''
			ORDER BY priority DESC, id ASC
			LIMIT 1`).Scan(&id, &desc, &sub, &kind)
	}
	if err != nil {
		return "", "", "", "", err
	}
	if _, err := tx.Exec(`UPDATE tasks SET status='in_progress', assigned_agent=?, updated_at=? WHERE id=?`,
		agentID, now, id); err != nil {
		return "", "", "", "", err
	}
	if _, err := tx.Exec(`INSERT INTO agents(id, status, last_heartbeat, created_at)
		VALUES(?, 'active', ?, ?)
		ON CONFLICT(id) DO UPDATE SET status='active', last_heartbeat=excluded.last_heartbeat`,
		agentID, now, now); err != nil {
		return "", "", "", "", err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO claims(agent, task_id, resource, resource_type, claimed_at, last_heartbeat, status)
		VALUES (?, ?, ?, 'task', ?, ?, 'active')`, agentID, id, id, now, now); err != nil {
		return "", "", "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", "", "", err
	}
	return id, sub, desc, kind, nil
}

// Claim reserves a repo+branch(+worktree) for an agent.
func (s *Store) Claim(agentID, resource, resourceType, branch, worktree, taskID string) error {
	if resourceType == "" {
		resourceType = "repo"
	}
	now := NowUTC()
	var existing string
	err := s.db.QueryRow(`SELECT agent FROM claims
		WHERE resource=? AND resource_type=? AND branch=? AND worktree=? AND status='active'`,
		resource, resourceType, branch, worktree).Scan(&existing)
	if err == nil {
		return ErrAlreadyClaimed{Resource: resource, ResourceType: resourceType}
	}
	if err != sql.ErrNoRows {
		return err
	}
	if _, err := s.db.Exec(`INSERT INTO claims(agent, task_id, resource, resource_type, branch, worktree, claimed_at, last_heartbeat, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active')`,
		agentID, taskID, resource, resourceType, branch, worktree, now, now); err != nil {
		return err
	}
	if _, err := s.db.Exec(`INSERT INTO agents(id, status, last_heartbeat, created_at)
		VALUES(?, 'active', ?, ?)
		ON CONFLICT(id) DO UPDATE SET status='active', last_heartbeat=excluded.last_heartbeat`,
		agentID, now, now); err != nil {
		return err
	}
	return nil
}

// MarkDone moves a task from in_progress → done and unblocks successors.
func (s *Store) MarkDone(agentID, taskID string) error {
	now := NowUTC()
	res, err := s.db.Exec(`UPDATE tasks SET status='done', updated_at=? WHERE id=? AND assigned_agent=?`,
		now, taskID, agentID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task %q not owned by %q", taskID, agentID)
	}
	rows, err := s.db.Query(`SELECT to_task FROM edges WHERE from_task=?`, taskID)
	if err != nil {
		return err
	}
	var successors []string
	for rows.Next() {
		var t string
		_ = rows.Scan(&t)
		successors = append(successors, t)
	}
	rows.Close()
	for _, s2 := range successors {
		preds, err := s.db.Query(`SELECT from_task FROM edges WHERE to_task=?`, s2)
		if err != nil {
			continue
		}
		allDone := true
		for preds.Next() {
			var p string
			_ = preds.Scan(&p)
			var st string
			_ = s.db.QueryRow(`SELECT status FROM tasks WHERE id=?`, p).Scan(&st)
			if st != "done" {
				allDone = false
				break
			}
		}
		preds.Close()
		if allDone {
			_, _ = s.db.Exec(`UPDATE tasks SET status='ready' WHERE id=? AND status='blocked'`, s2)
		}
	}
	return nil
}

// Heartbeat updates the agent's last_heartbeat.
func (s *Store) Heartbeat(agentID string) error {
	now := NowUTC()
	_, err := s.db.Exec(`INSERT INTO agents(id, status, last_heartbeat, created_at)
		VALUES(?, 'active', ?, ?)
		ON CONFLICT(id) DO UPDATE SET status='active', last_heartbeat=excluded.last_heartbeat`,
		agentID, now, now)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE claims SET last_heartbeat=? WHERE agent=? AND status='active'`, now, agentID)
	return err
}

// ReclaimStale releases claims whose last_heartbeat is older than staleMin minutes.
func (s *Store) ReclaimStale(staleMin int) (int, error) {
	now := NowUTC()
	res, err := s.db.Exec(`UPDATE claims SET status='reaped'
		WHERE status='active'
		AND (julianday(?) - julianday(last_heartbeat)) * 24 * 60 > ?`, now, staleMin)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		_, _ = s.db.Exec(`UPDATE tasks SET status='ready', assigned_agent='', updated_at=?
			WHERE status='in_progress' AND assigned_agent IN
			(SELECT agent FROM claims WHERE status='reaped' AND last_heartbeat=?)`, now, now)
	}
	return int(n), nil
}
