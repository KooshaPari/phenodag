package remoteclaim

import (
	"context"
	"time"
)

// Store is the local persistence layer for remote claims (AgilePlus semantics).
type Store interface {
	Claim(ctx context.Context, c Claim) (*Claim, error)
	Heartbeat(ctx context.Context, claimID, agentID string, seenEpoch int64) error
	Release(ctx context.Context, claimID, agentID string, seenEpoch int64) error
	ForceRelease(ctx context.Context, claimID, reason string) error
	ReapExpired(ctx context.Context, now time.Time) ([]Claim, error)
	Transfer(ctx context.Context, fromID, toID, toAgent string) (*Claim, error)
	LookupActive(ctx context.Context, kind Kind, repo, branch, worktree string) (*Claim, error)
	Get(ctx context.Context, claimID string) (*Claim, error)
	Active(ctx context.Context) ([]Claim, error)
}

// Transport coordinates claims across hosts (local store or GitHub).
type Transport interface {
	Claim(ctx context.Context, c Claim) (*Claim, error)
	Heartbeat(ctx context.Context, claimID, agentID string, seenEpoch int64) error
	Release(ctx context.Context, claimID, agentID string, seenEpoch int64) error
	Transfer(ctx context.Context, fromID, toID, toAgent string) error
	ReapExpired(ctx context.Context, now time.Time) ([]Claim, error)
	List(ctx context.Context) ([]Claim, error)
}
