// Package claim is the unified claim facade for the phenodag + dagctl
// superset-merge (Phase-4b, issue #5).
//
// Two claim systems coexist in the merged binary:
//
//   - internal/remoteclaim (dagctl's local-SQLite + GitHub transport).
//     Used for inter-host coordination via local DB and a tracking
//     GitHub issue.
//   - gh_repo_lease.go (phenodag's gh-cli multi-chat lease store).
//     Used for cross-chat coordination of the same repo across many
//     AI agent sessions, with GitHub Issues used as the audit trail.
//
// The ClaimStore interface in this package is the smallest surface
// required by all 38+ claim/lease-related commands. Implementations
// delegate to whichever underlying transport is appropriate:
//
//   - NewRemoteStore wraps internal/remoteclaim.Transport.
//   - NewLeaseStore wraps gh_repo_lease.SQLiteLeaseStore.
//
// Pick() returns the right one for a given command flag (--transport=
// local|github|lease); the cmd* funcs in main stay transport-agnostic.
//
// See ADR-dag-superset-merge.md and ADR-dedup-baseline.md.
package claim

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Kind is the resource granularity. Mirrors remoteclaim.Kind and the
// LeaseKind constants in gh_repo_lease.go (kept compatible for JSON
// round-tripping).
type Kind string

const (
	KindRepo      Kind = "repo"
	KindBranch    Kind = "branch"
	KindWorktree  Kind = "worktree"
	KindSubrepo   Kind = "subrepo"
	KindSubproject Kind = "subproject"
)

// State tracks claim lifecycle.
type State string

const (
	StateActive   State = "active"
	StateDraining State = "draining"
	StateExpired  State = "expired"
)

// Claim is the unified claim/lease projection used by cmd funcs.
type Claim struct {
	ID            string
	Kind          Kind
	Resource      string
	Repo          string
	Branch        string
	Worktree      string
	State         State
	AgentID       string
	TTLSeconds    int64
	Epoch         int64
	TaskID        string
	ReasonKind    string
	ReasonValue   string
	AcquiredAt    time.Time
	LastHeartbeat time.Time
}

// ClaimStore is the smallest interface that covers all 6 remoteclaim
// ops (Claim/Heartbeat/Release/Transfer/ReapExpired/List) and the 6
// gh_repo_lease ops (Acquire/Heartbeat/Release/Reap/Transfer/Get/List).
type ClaimStore interface {
	Claim(ctx context.Context, c Claim) (*Claim, error)
	Heartbeat(ctx context.Context, id, agentID string, epoch int64) error
	Release(ctx context.Context, id, agentID string, epoch int64) error
	Transfer(ctx context.Context, fromID, toID, toAgent string) error
	ReapExpired(ctx context.Context) ([]Claim, error)
	List(ctx context.Context) ([]Claim, error)
}

// ErrConflict is returned when a resource is already held.
var ErrConflict = errors.New("claim conflict: resource already held")

// ErrNotFound is returned when a claim id does not exist.
var ErrNotFound = errors.New("claim not found")

// ParseOwnerRepo splits "owner/name" into (owner, name).
func ParseOwnerRepo(slug string) (owner, name string, err error) {
	idx := strings.Index(slug, "/")
	if idx < 1 || idx == len(slug)-1 {
		return "", "", fmt.Errorf("expected owner/repo slug, got %q", slug)
	}
	return slug[:idx], slug[idx+1:], nil
}

// ResourceKey is the canonical resource string for a (kind, repo,
// branch, worktree) tuple. Mirrors internal/remoteclaim.ResourceKey
// so the two systems produce identical keys when wrapping the same
// (kind, repo, branch, worktree).
func ResourceKey(kind Kind, repo, branch, worktree string) string {
	switch kind {
	case KindRepo:
		return "repo:" + repo
	case KindBranch:
		return "branch:" + repo + ":" + branch
	case KindWorktree:
		return "worktree:" + repo + ":" + branch + ":" + worktree
	case KindSubproject, KindSubrepo:
		return "subproject:" + repo
	default:
		return string(kind) + ":" + repo
	}
}
