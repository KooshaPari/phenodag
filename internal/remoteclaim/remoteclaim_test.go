package remoteclaim

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestClaimAndHeartbeat(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	transport := NewLocalTransport(store)
	issued, err := transport.Claim(ctx, Claim{
		AgentID: "chat-A",
		Kind:    KindRepo,
		Repo:    "HexaKit",
		TTLSeconds: 3600,
		Reason:  Reason{Kind: ReasonTaskRef, Value: "wp-1"},
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if issued.ID == "" {
		t.Fatal("expected claim id")
	}
	if issued.Epoch != 1 {
		t.Fatalf("epoch=%d want 1", issued.Epoch)
	}

	if err := transport.Heartbeat(ctx, issued.ID, "chat-A", issued.Epoch); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	got, err := store.Get(ctx, issued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastHeartbeat.After(issued.LastHeartbeat) && got.LastHeartbeat.Equal(issued.LastHeartbeat) {
		t.Fatal("heartbeat should refresh last_heartbeat")
	}
}

func TestClaimConflict(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	transport := NewLocalTransport(store)

	_, err = transport.Claim(ctx, Claim{AgentID: "a1", Kind: KindRepo, Repo: "foo"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = transport.Claim(ctx, Claim{AgentID: "a2", Kind: KindRepo, Repo: "foo"})
	if err != ErrConflict {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestReapExpired(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	transport := NewLocalTransport(store)

	past := time.Now().UTC().Add(-2 * time.Hour)
	issued, err := store.Claim(ctx, Claim{
		AgentID: "a1", Kind: KindRepo, Repo: "stale",
		TTLSeconds: 60, LastHeartbeat: past, CreatedAt: past,
	})
	if err != nil {
		t.Fatal(err)
	}

	reaped, err := transport.ReapExpired(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(reaped) != 1 || reaped[0].ID != issued.ID {
		t.Fatalf("reaped=%+v want %s", reaped, issued.ID)
	}
	active, _ := store.Active(ctx)
	if len(active) != 0 {
		t.Fatalf("expected 0 active, got %d", len(active))
	}
}

func TestFencingRejectsStaleHeartbeat(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	issued, err := store.Claim(ctx, Claim{AgentID: "zombie", Kind: KindRepo, Repo: "fence"})
	if err != nil {
		t.Fatal(err)
	}
	staleEpoch := issued.Epoch

	// Simulate transfer bumping epoch.
	_, err = store.Transfer(ctx, issued.ID, "", "new-agent")
	if err != nil {
		t.Fatal(err)
	}

	err = store.Heartbeat(ctx, issued.ID, "zombie", staleEpoch)
	if err != ErrStaleEpoch {
		t.Fatalf("want ErrStaleEpoch, got %v", err)
	}
}

func TestTransfer(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	transport := NewLocalTransport(store)

	old, err := transport.Claim(ctx, Claim{AgentID: "a1", Kind: KindRepo, Repo: "handoff", TTLSeconds: 120})
	if err != nil {
		t.Fatal(err)
	}
	if err := transport.Transfer(ctx, old.ID, "c-new", "a2"); err != nil {
		t.Fatal(err)
	}
	newClaim, err := store.Get(ctx, "c-new")
	if err != nil {
		t.Fatal(err)
	}
	if newClaim.AgentID != "a2" || newClaim.Epoch != old.Epoch+1 {
		t.Fatalf("new claim=%+v", newClaim)
	}
	oldRow, _ := store.Get(ctx, old.ID)
	if oldRow.Status != StateDraining {
		t.Fatalf("old status=%s want draining", oldRow.Status)
	}
}

func TestReleaseWithFencing(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	c, err := store.Claim(ctx, Claim{AgentID: "owner", Kind: KindRepo, Repo: "rel"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Release(ctx, c.ID, "owner", c.Epoch); err != nil {
		t.Fatal(err)
	}
	err = store.Heartbeat(ctx, c.ID, "owner", c.Epoch)
	if err != ErrStaleEpoch {
		t.Fatalf("want ErrStaleEpoch after release, got %v", err)
	}
}

func TestGitHubTransportMock(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	mock := &mockGH{}
	gh := NewGitHubTransport(GitHubConfig{Owner: "o", Repo: "r", IssueNumber: 1}, store, mock)
	issued, err := gh.Claim(ctx, Claim{AgentID: "chat-A", Kind: KindRepo, Repo: "remote-repo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(mock.labels) != 1 || mock.labels[0] != "claim:repo/remote-repo" {
		t.Fatalf("labels=%v", mock.labels)
	}
	if len(mock.comments) != 1 {
		t.Fatal("expected claim comment")
	}
	var ev Event
	if err := json.Unmarshal([]byte(mock.comments[0]), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Event != "claim" || ev.ClaimID != issued.ID {
		t.Fatalf("event=%+v", ev)
	}

	if err := gh.Heartbeat(ctx, issued.ID, "chat-A", issued.Epoch); err != nil {
		t.Fatal(err)
	}
	if len(mock.comments) != 2 {
		t.Fatalf("comments=%d want 2", len(mock.comments))
	}
}

func TestParseOwnerRepo(t *testing.T) {
	owner, name, err := ParseOwnerRepo("KooshaPari/phenodag")
	if err != nil || owner != "KooshaPari" || name != "phenodag" {
		t.Fatalf("got %s/%s err=%v", owner, name, err)
	}
	_, _, err = ParseOwnerRepo("invalid")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWithLock(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/claims.db"
	store, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	_, err = store.Claim(ctx, Claim{AgentID: "a", Kind: KindRepo, Repo: "lock-test"})
	if err != nil {
		t.Fatal(err)
	}
}

type mockGH struct {
	labels   []string
	comments []string
}

func (m *mockGH) Run(ctx context.Context, args ...string) ([]byte, error) {
	joined := ""
	for _, a := range args {
		joined += a + " "
	}
	if contains(args, "POST") && contains(args, "labels") {
		for i, a := range args {
			if stringsHasPrefix(a, "labels[]=") {
				m.labels = append(m.labels, a[len("labels[]="):])
			}
			_ = i
		}
		return []byte(`[]`), nil
	}
	if contains(args, "POST") && contains(args, "comments") {
		for _, a := range args {
			if stringsHasPrefix(a, "body=") {
				m.comments = append(m.comments, a[len("body="):])
			}
		}
		return []byte(`{"id":1}`), nil
	}
	if contains(args, "comments?") {
		var bodies []map[string]string
		for _, c := range m.comments {
			bodies = append(bodies, map[string]string{"body": c})
		}
		out, _ := json.Marshal(bodies)
		return out, nil
	}
	if contains(args, "issues/1") && !contains(args, "comments") && !contains(args, "labels") {
		return []byte(`{"labels":[]}`), nil
	}
	return []byte(`{}`), nil
}

func contains(args []string, sub string) bool {
	for _, a := range args {
		if stringsContains(a, sub) {
			return true
		}
	}
	return false
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func stringsContains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
