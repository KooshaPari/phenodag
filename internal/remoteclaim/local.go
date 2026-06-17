package remoteclaim

import (
	"context"
	"time"
)

// LocalTransport round-trips to the SQLite store (default --transport=local).
type LocalTransport struct {
	store *SQLiteStore
}

func NewLocalTransport(store *SQLiteStore) *LocalTransport {
	return &LocalTransport{store: store}
}

func (t *LocalTransport) Claim(ctx context.Context, c Claim) (*Claim, error) {
	return t.store.Claim(ctx, c)
}

func (t *LocalTransport) Heartbeat(ctx context.Context, claimID, agentID string, seenEpoch int64) error {
	return t.store.Heartbeat(ctx, claimID, agentID, seenEpoch)
}

func (t *LocalTransport) Release(ctx context.Context, claimID, agentID string, seenEpoch int64) error {
	return t.store.Release(ctx, claimID, agentID, seenEpoch)
}

func (t *LocalTransport) Transfer(ctx context.Context, fromID, toID, toAgent string) error {
	_, err := t.store.Transfer(ctx, fromID, toID, toAgent)
	return err
}

func (t *LocalTransport) ReapExpired(ctx context.Context, now time.Time) ([]Claim, error) {
	return t.store.ReapExpired(ctx, now)
}

func (t *LocalTransport) List(ctx context.Context) ([]Claim, error) {
	return t.store.Active(ctx)
}
