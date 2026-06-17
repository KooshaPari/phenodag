package remoteclaim

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const DefaultTTLSeconds = 3600

// SQLiteStore persists remote claims with TTL, heartbeat, and fencing epoch.
type SQLiteStore struct {
	db   *sql.DB
	path string
}

// OpenSQLite opens (or creates) a remote-claims SQLite database.
func OpenSQLite(path string) (*SQLiteStore, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &SQLiteStore{db: db, path: path}
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// OpenSQLiteMemory opens an in-memory store for tests.
func OpenSQLiteMemory() (*SQLiteStore, error) {
	dsn := "file:remoteclaim_mem?mode=memory&cache=shared&_pragma=foreign_keys(on)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &SQLiteStore{db: db, path: ":memory:"}
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) Path() string { return s.path }

func (s *SQLiteStore) initSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS remote_claims (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			resource TEXT NOT NULL,
			kind TEXT NOT NULL,
			repo TEXT NOT NULL,
			branch TEXT NOT NULL DEFAULT '',
			worktree TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			reason_kind TEXT,
			reason_value TEXT,
			created_at TEXT NOT NULL,
			last_heartbeat TEXT NOT NULL,
			ttl_seconds INTEGER NOT NULL DEFAULT 3600,
			epoch INTEGER NOT NULL DEFAULT 1,
			task_id TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_remote_claims_active_resource
			ON remote_claims(repo, branch, worktree) WHERE status = 'active'`,
		`CREATE INDEX IF NOT EXISTS idx_remote_claims_agent ON remote_claims(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_remote_claims_status ON remote_claims(status)`,
		`CREATE INDEX IF NOT EXISTS idx_remote_claims_hb ON remote_claims(last_heartbeat)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// NewClaimID returns a short unique claim id.
func NewClaimID() string {
	return "c-" + uuid.New().String()[:8]
}

func (s *SQLiteStore) Claim(ctx context.Context, c Claim) (*Claim, error) {
	return s.claimLocked(ctx, c)
}

func (s *SQLiteStore) claimLocked(ctx context.Context, c Claim) (*Claim, error) {
	var out *Claim
	err := WithLock(s.path, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		var existingID string
		err = tx.QueryRowContext(ctx,
			`SELECT id FROM remote_claims WHERE repo=? AND branch=? AND worktree=? AND status='active'`,
			c.Repo, c.Branch, c.Worktree,
		).Scan(&existingID)
		if err == nil && existingID != c.ID {
			return ErrConflict
		}
		if err != nil && err != sql.ErrNoRows {
			return err
		}

		now := time.Now().UTC()
		if c.ID == "" {
			c.ID = NewClaimID()
		}
		if c.CreatedAt.IsZero() {
			c.CreatedAt = now
		}
		if c.LastHeartbeat.IsZero() {
			c.LastHeartbeat = now
		}
		if c.TTLSeconds == 0 {
			c.TTLSeconds = DefaultTTLSeconds
		}
		if c.Status == "" {
			c.Status = StateActive
		}
		if c.Epoch == 0 {
			c.Epoch = 1
		}
		if c.Resource == "" {
			c.Resource = ResourceKey(c.Kind, c.Repo, c.Branch, c.Worktree)
		}

		_, err = tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO remote_claims
			(id, agent_id, resource, kind, repo, branch, worktree, status,
			 reason_kind, reason_value, created_at, last_heartbeat, ttl_seconds, epoch, task_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, c.AgentID, c.Resource, string(c.Kind), c.Repo, c.Branch, c.Worktree, string(c.Status),
			string(c.Reason.Kind), c.Reason.Value,
			c.CreatedAt.Format(time.RFC3339), c.LastHeartbeat.Format(time.RFC3339),
			c.TTLSeconds, c.Epoch, c.TaskID,
		)
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		cp := c
		out = &cp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) Heartbeat(ctx context.Context, claimID, agentID string, seenEpoch int64) error {
	return WithLock(s.path, func() error {
		now := time.Now().UTC().Format(time.RFC3339)
		res, err := s.db.ExecContext(ctx, `
			UPDATE remote_claims SET last_heartbeat=?
			WHERE id=? AND agent_id=? AND epoch=? AND status='active'`,
			now, claimID, agentID, seenEpoch,
		)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			var exists int
			_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM remote_claims WHERE id=?`, claimID).Scan(&exists)
			if exists == 0 {
				return ErrNotFound
			}
			return ErrStaleEpoch
		}
		return nil
	})
}

func (s *SQLiteStore) Release(ctx context.Context, claimID, agentID string, seenEpoch int64) error {
	return WithLock(s.path, func() error {
		res, err := s.db.ExecContext(ctx, `
			UPDATE remote_claims SET status='expired', epoch=epoch+1
			WHERE id=? AND agent_id=? AND epoch=? AND status IN ('active','draining')`,
			claimID, agentID, seenEpoch,
		)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			var owner string
			err := s.db.QueryRowContext(ctx, `SELECT agent_id FROM remote_claims WHERE id=?`, claimID).Scan(&owner)
			if err == sql.ErrNoRows {
				return ErrNotFound
			}
			if owner != agentID {
				return ErrWrongOwner
			}
			return ErrStaleEpoch
		}
		return nil
	})
}

func (s *SQLiteStore) ForceRelease(ctx context.Context, claimID, reason string) error {
	_ = reason
	return WithLock(s.path, func() error {
		res, err := s.db.ExecContext(ctx, `
			UPDATE remote_claims SET status='expired', epoch=epoch+1
			WHERE id=? AND status IN ('active','draining')`, claimID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *SQLiteStore) ReapExpired(ctx context.Context, now time.Time) ([]Claim, error) {
	var reaped []Claim
	err := WithLock(s.path, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, agent_id, resource, kind, repo, branch, worktree, status,
			       reason_kind, reason_value, created_at, last_heartbeat, ttl_seconds, epoch, task_id
			FROM remote_claims WHERE status IN ('active','draining')`)
		if err != nil {
			return err
		}
		defer rows.Close()

		var toReap []string
		for rows.Next() {
			c, err := scanClaim(rows)
			if err != nil {
				return err
			}
			if c.IsExpired(now) {
				toReap = append(toReap, c.ID)
				c.Status = StateExpired
				reaped = append(reaped, c)
			}
		}
		for _, id := range toReap {
			_, err := s.db.ExecContext(ctx, `
				UPDATE remote_claims SET status='expired', epoch=epoch+1 WHERE id=?`, id)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return reaped, err
}

func (s *SQLiteStore) Transfer(ctx context.Context, fromID, toID, toAgent string) (*Claim, error) {
	var out *Claim
	err := WithLock(s.path, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		old, err := getClaimTx(ctx, tx, fromID)
		if err != nil {
			return err
		}
		if old.Status != StateActive {
			return ErrWrongState
		}

		_, err = tx.ExecContext(ctx, `
			UPDATE remote_claims SET status='draining', epoch=epoch+1 WHERE id=? AND status='active'`,
			fromID)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		if toID == "" {
			toID = NewClaimID()
		}
		newClaim := Claim{
			ID:            toID,
			AgentID:       toAgent,
			Resource:      old.Resource,
			Kind:          old.Kind,
			Repo:          old.Repo,
			Branch:        old.Branch,
			Worktree:      old.Worktree,
			Status:        StateActive,
			Reason:        old.Reason,
			CreatedAt:     now,
			LastHeartbeat: now,
			TTLSeconds:    old.TTLSeconds,
			Epoch:         old.Epoch + 1,
			TaskID:        old.TaskID,
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO remote_claims
			(id, agent_id, resource, kind, repo, branch, worktree, status,
			 reason_kind, reason_value, created_at, last_heartbeat, ttl_seconds, epoch, task_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			newClaim.ID, newClaim.AgentID, newClaim.Resource, string(newClaim.Kind),
			newClaim.Repo, newClaim.Branch, newClaim.Worktree, string(newClaim.Status),
			string(newClaim.Reason.Kind), newClaim.Reason.Value,
			newClaim.CreatedAt.Format(time.RFC3339), newClaim.LastHeartbeat.Format(time.RFC3339),
			newClaim.TTLSeconds, newClaim.Epoch, newClaim.TaskID,
		)
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		out = &newClaim
		return nil
	})
	return out, err
}

func (s *SQLiteStore) LookupActive(ctx context.Context, kind Kind, repo, branch, worktree string) (*Claim, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, resource, kind, repo, branch, worktree, status,
		       reason_kind, reason_value, created_at, last_heartbeat, ttl_seconds, epoch, task_id
		FROM remote_claims WHERE repo=? AND branch=? AND worktree=? AND status='active'`,
		repo, branch, worktree)
	c, err := scanClaimRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = kind
	return c, nil
}

func (s *SQLiteStore) Get(ctx context.Context, claimID string) (*Claim, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, resource, kind, repo, branch, worktree, status,
		       reason_kind, reason_value, created_at, last_heartbeat, ttl_seconds, epoch, task_id
		FROM remote_claims WHERE id=?`, claimID)
	return scanClaimRow(row)
}

func (s *SQLiteStore) Active(ctx context.Context) ([]Claim, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, resource, kind, repo, branch, worktree, status,
		       reason_kind, reason_value, created_at, last_heartbeat, ttl_seconds, epoch, task_id
		FROM remote_claims WHERE status='active' ORDER BY repo, branch`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Claim
	for rows.Next() {
		c, err := scanClaim(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func getClaimTx(ctx context.Context, tx *sql.Tx, id string) (*Claim, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, agent_id, resource, kind, repo, branch, worktree, status,
		       reason_kind, reason_value, created_at, last_heartbeat, ttl_seconds, epoch, task_id
		FROM remote_claims WHERE id=?`, id)
	return scanClaimRow(row)
}

type claimScanner interface {
	Scan(dest ...any) error
}

func scanClaim(rows claimScanner) (Claim, error) {
	var c Claim
	var kind, status, reasonKind, createdAt, lastHB string
	var reasonValue sql.NullString
	err := rows.Scan(
		&c.ID, &c.AgentID, &c.Resource, &kind, &c.Repo, &c.Branch, &c.Worktree, &status,
		&reasonKind, &reasonValue, &createdAt, &lastHB, &c.TTLSeconds, &c.Epoch, &c.TaskID,
	)
	if err != nil {
		return Claim{}, err
	}
	c.Kind = Kind(kind)
	c.Status = State(status)
	c.Reason = Reason{Kind: ReasonKind(reasonKind), Value: reasonValue.String}
	c.CreatedAt = parseTime(createdAt)
	c.LastHeartbeat = parseTime(lastHB)
	return c, nil
}

func scanClaimRow(row *sql.Row) (*Claim, error) {
	c, err := scanClaim(row)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
