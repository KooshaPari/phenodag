package claim

import (
	"context"

	"github.com/KooshaPari/phenodag/internal/remoteclaim"
)

// RemoteStore is the ClaimStore implementation that delegates to
// internal/remoteclaim (dagctl's local + github transport).
type RemoteStore struct {
	Transport remoteclaim.Transport
}

// NewRemoteStore wraps an already-constructed remoteclaim.Transport.
func NewRemoteStore(t remoteclaim.Transport) *RemoteStore {
	return &RemoteStore{Transport: t}
}

func (s *RemoteStore) Claim(ctx context.Context, c Claim) (*Claim, error) {
	kind := remoteclaim.Kind(string(c.Kind))
	reason := remoteclaim.Reason{
		Kind:  remoteclaim.ReasonKind(orDefault(c.ReasonKind, string(remoteclaim.ReasonManual))),
		Value: c.ReasonValue,
	}
	rc := remoteclaim.Claim{
		AgentID:    c.AgentID,
		Kind:       kind,
		Repo:       c.Repo,
		Branch:     c.Branch,
		Worktree:   c.Worktree,
		TTLSeconds: c.TTLSeconds,
		TaskID:     c.TaskID,
		Reason:     reason,
	}
	if rc.Resource == "" {
		rc.Resource = remoteclaim.ResourceKey(kind, c.Repo, c.Branch, c.Worktree)
	}
	issued, err := s.Transport.Claim(ctx, rc)
	if err != nil {
		return nil, err
	}
	return toUnified(issued), nil
}

func (s *RemoteStore) Heartbeat(ctx context.Context, id, agentID string, epoch int64) error {
	return s.Transport.Heartbeat(ctx, id, agentID, epoch)
}

func (s *RemoteStore) Release(ctx context.Context, id, agentID string, epoch int64) error {
	return s.Transport.Release(ctx, id, agentID, epoch)
}

func (s *RemoteStore) Transfer(ctx context.Context, fromID, toID, toAgent string) error {
	return s.Transport.Transfer(ctx, fromID, toID, toAgent)
}

func (s *RemoteStore) ReapExpired(ctx context.Context) ([]Claim, error) {
	rs, err := s.Transport.ReapExpired(ctx, timeNow())
	if err != nil {
		return nil, err
	}
	out := make([]Claim, 0, len(rs))
	for i := range rs {
		out = append(out, *toUnified(&rs[i]))
	}
	return out, nil
}

func (s *RemoteStore) List(ctx context.Context) ([]Claim, error) {
	rs, err := s.Transport.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Claim, 0, len(rs))
	for i := range rs {
		out = append(out, *toUnified(&rs[i]))
	}
	return out, nil
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func toUnified(c *remoteclaim.Claim) *Claim {
	return &Claim{
		ID:            c.ID,
		Kind:          Kind(string(c.Kind)),
		Resource:      c.Resource,
		Repo:          c.Repo,
		Branch:        c.Branch,
		Worktree:      c.Worktree,
		State:         State(string(c.Status)),
		AgentID:       c.AgentID,
		TTLSeconds:    c.TTLSeconds,
		Epoch:         c.Epoch,
		TaskID:        c.TaskID,
		ReasonKind:    string(c.Reason.Kind),
		ReasonValue:   c.Reason.Value,
		AcquiredAt:    c.CreatedAt,
		LastHeartbeat: c.LastHeartbeat,
	}
}
