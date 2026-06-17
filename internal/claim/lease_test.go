package claim

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubLease struct {
	leases map[string]*leaseRecord
}

func newStubLease() *stubLease {
	return &stubLease{leases: map[string]*leaseRecord{}}
}

func (s *stubLease) Acquire(ctx context.Context, l *leaseRecord) error {
	for _, r := range s.leases {
		if r.Resource == l.Resource && r.State == string(StateActive) {
			return errors.New("conflict")
		}
	}
	if l.ID == "" {
		l.ID = "lease-test"
	}
	l.State = string(StateActive)
	l.AcquiredAt = time.Unix(0, 0)
	l.LastHeartbeat = l.AcquiredAt
	l.EpochToken = l.AcquiredAt.Unix()
	s.leases[l.ID] = l
	return nil
}

func (s *stubLease) Heartbeat(ctx context.Context, id, chatID string, epoch int64) error {
	r, ok := s.leases[id]
	if !ok {
		return ErrNotFound
	}
	r.LastHeartbeat = time.Unix(0, 0)
	r.EpochToken = epoch
	return nil
}

func (s *stubLease) Release(ctx context.Context, id, chatID string) error {
	r, ok := s.leases[id]
	if !ok {
		return ErrNotFound
	}
	r.State = string(StateExpired)
	return nil
}

func (s *stubLease) ReapExpired(ctx context.Context) (int, error) {
	n := 0
	for _, r := range s.leases {
		if r.State == string(StateActive) {
			r.State = string(StateExpired)
			n++
		}
	}
	return n, nil
}

func (s *stubLease) Transfer(ctx context.Context, id, oldChatID, newChatID string, epoch int64) error {
	r, ok := s.leases[id]
	if !ok {
		return ErrNotFound
	}
	r.ChatID = newChatID
	r.EpochToken = epoch
	return nil
}

func (s *stubLease) Get(ctx context.Context, id string) (*leaseRecord, error) {
	r, ok := s.leases[id]
	if !ok {
		return nil, ErrNotFound
	}
	return r, nil
}

func (s *stubLease) ListActive(ctx context.Context) ([]*leaseRecord, error) {
	out := make([]*leaseRecord, 0)
	for _, r := range s.leases {
		if r.State == string(StateActive) {
			out = append(out, r)
		}
	}
	return out, nil
}

func TestLeaseStoreClaimAndRelease(t *testing.T) {
	s := NewLeaseStore(newStubLease())
	c, err := s.Claim(context.Background(), Claim{
		Kind:     KindRepo,
		Resource: "repo:test",
		AgentID:  "chat-1",
	})
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if c.ID == "" {
		t.Errorf("Claim ID should be set")
	}
	if err := s.Release(context.Background(), c.ID, "chat-1", 0); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestResourceKeyUnified(t *testing.T) {
	cases := []struct {
		kind                Kind
		repo, branch, wt    string
		want                string
	}{
		{KindRepo, "phenodag", "", "", "repo:phenodag"},
		{KindBranch, "phenodag", "feat/x", "", "branch:phenodag:feat/x"},
		{KindWorktree, "phenodag", "feat/x", "wt-1", "worktree:phenodag:feat/x:wt-1"},
		{KindSubproject, "phenodag", "", "", "subproject:phenodag"},
	}
	for _, c := range cases {
		got := ResourceKey(c.kind, c.repo, c.branch, c.wt)
		if got != c.want {
			t.Errorf("ResourceKey(%q,%q,%q,%q) = %q, want %q",
				c.kind, c.repo, c.branch, c.wt, got, c.want)
		}
	}
}

func TestParseOwnerRepo(t *testing.T) {
	o, n, err := ParseOwnerRepo("KooshaPari/phenodag")
	if err != nil {
		t.Fatalf("ParseOwnerRepo: %v", err)
	}
	if o != "KooshaPari" || n != "phenodag" {
		t.Errorf("got (%q,%q), want (KooshaPari,phenodag)", o, n)
	}
	if _, _, err := ParseOwnerRepo("nope"); err == nil {
		t.Errorf("expected error for missing slash")
	}
}
