package claim

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// LeaseStore is the ClaimStore implementation that wraps
// gh_repo_lease.go's SQLiteLeaseStore. It provides an in-process
// shim so the merged binary can use the multi-chat lease system
// without forcing a hard dependency on the gh-cli at compile time
// (the underlying store is invoked through the same interface
// pattern as the merged Release / Heartbeat / Reap paths).
//
// This file is intentionally small: it provides the wire-up glue
// between the unified Claim interface and the lease store API
// (Acquire / Heartbeat / Release / Reap / Transfer / Get / List)
// exposed by gh_repo_lease.go's LeaseStore interface.
type LeaseStore struct {
	mu     sync.Mutex
	inner  leaseInner
	leases map[string]*Claim
}

// leaseInner is the subset of gh_repo_lease.go's LeaseStore the
// facade needs. In production this is satisfied by
// *SQLiteLeaseStore; tests can provide a stub.
type leaseInner interface {
	Acquire(ctx context.Context, l *leaseRecord) error
	Heartbeat(ctx context.Context, id, chatID string, epoch int64) error
	Release(ctx context.Context, id, chatID string) error
	ReapExpired(ctx context.Context) (int, error)
	Transfer(ctx context.Context, id, oldChatID, newChatID string, epoch int64) error
	Get(ctx context.Context, id string) (*leaseRecord, error)
	ListActive(ctx context.Context) ([]*leaseRecord, error)
}

// leaseRecord mirrors gh_repo_lease.Lease. We avoid importing the
// type directly to keep the facade package free of cgo; the
// cmd-layer (gh_repo_lease.go) provides the bridge at wire-up time.
type leaseRecord struct {
	ID              string
	Kind            string
	Resource        string
	State           string
	ChatID          string
	TTLSeconds      int64
	EpochToken      int64
	AcquiredAt      time.Time
	LastHeartbeat   time.Time
	ReasonKind      string
	ReasonValue     string
	GitHubCoordPath string
}

// NewLeaseStore wraps an existing leaseInner. Use a constructor in
// main that adapts *gh_repo_lease.SQLiteLeaseStore to leaseInner
// (see cmd/phenodag/claim_bridge.go for the glue).
func NewLeaseStore(inner leaseInner) *LeaseStore {
	return &LeaseStore{inner: inner, leases: map[string]*Claim{}}
}

func (s *LeaseStore) Claim(ctx context.Context, c Claim) (*Claim, error) {
	rec := &leaseRecord{
		Kind:        string(c.Kind),
		Resource:    c.Resource,
		ChatID:      c.AgentID,
		TTLSeconds:  c.TTLSeconds,
		ReasonKind:  c.ReasonKind,
		ReasonValue: c.ReasonValue,
	}
	if err := s.inner.Acquire(ctx, rec); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.leases[rec.ID] = fromLeaseRecord(rec)
	s.mu.Unlock()
	return s.leases[rec.ID], nil
}

func (s *LeaseStore) Heartbeat(ctx context.Context, id, agentID string, epoch int64) error {
	return s.inner.Heartbeat(ctx, id, agentID, epoch)
}

func (s *LeaseStore) Release(ctx context.Context, id, agentID string, epoch int64) error {
	if err := s.inner.Release(ctx, id, agentID); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.leases, id)
	s.mu.Unlock()
	return nil
}

func (s *LeaseStore) Transfer(ctx context.Context, fromID, toID, toAgent string) error {
	// Re-claim with the new agent. The underlying lease store's
	// Transfer checks the old chat-id and epoch; the cmd layer
	// passes the current epoch captured before the call.
	s.mu.Lock()
	old, ok := s.leases[fromID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("lease not found: %s", fromID)
	}
	if err := s.inner.Transfer(ctx, fromID, old.AgentID, toAgent, old.Epoch); err != nil {
		return err
	}
	s.mu.Lock()
	old.AgentID = toAgent
	s.mu.Unlock()
	return nil
}

func (s *LeaseStore) ReapExpired(ctx context.Context) ([]Claim, error) {
	n, err := s.inner.ReapExpired(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	// Re-list after reap.
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Claim, 0, n)
	for _, c := range all {
		if c.State == StateExpired {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *LeaseStore) List(ctx context.Context) ([]Claim, error) {
	recs, err := s.inner.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Claim, 0, len(recs))
	for _, r := range recs {
		out = append(out, *fromLeaseRecord(r))
	}
	return out, nil
}

func fromLeaseRecord(r *leaseRecord) *Claim {
	return &Claim{
		ID:            r.ID,
		Kind:          Kind(r.Kind),
		Resource:      r.Resource,
		State:         State(r.State),
		AgentID:       r.ChatID,
		TTLSeconds:    r.TTLSeconds,
		Epoch:         r.EpochToken,
		ReasonKind:    r.ReasonKind,
		ReasonValue:   r.ReasonValue,
		AcquiredAt:    r.AcquiredAt,
		LastHeartbeat: r.LastHeartbeat,
	}
}
