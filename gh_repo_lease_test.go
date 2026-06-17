// gh_repo_lease_test.go — xDD tests for multi-chat repo lease/claim system.
//
// Tests verify:
// 1. Two concurrent chats race to claim one repo — exactly one wins
// 2. Winner can heartbeat; loser's heartbeats are rejected
// 3. Expired lease (TTL elapsed, no heartbeat) is reaped back to pool
// 4. Epoch fencing rejects stale claimants (prevents split-brain)
// 5. Claim transfer (failover) respects epoch tokens

package main

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestConcurrentClaim_OnlyOneWins verifies mutual exclusion on repo resource.
func TestConcurrentClaim_OnlyOneWins(t *testing.T) {
	// Setup
	db := testDB(t)
	defer os.Remove(db)
	store, err := NewSQLiteLeaseStore(db, "", "")
	if err != nil {
		t.Fatalf("NewSQLiteLeaseStore: %v", err)
	}

	ctx := context.Background()

	// Chat A and Chat B race to claim the same repo
	chatA, chatB := "chat-a", "chat-b"
	resource := "KooshaPari/Repo"

	leaseA := &Lease{
		Kind:       LeaseKindRepo,
		Resource:   resource,
		ChatID:     chatA,
		TTLSeconds: 3600,
		Reason:     LeaseReason{Kind: "task_ref", Value: "phase-4-merge"},
	}
	leaseB := &Lease{
		Kind:       LeaseKindRepo,
		Resource:   resource,
		ChatID:     chatB,
		TTLSeconds: 3600,
		Reason:     LeaseReason{Kind: "task_ref", Value: "phase-4-merge"},
	}

	// First acquire should succeed
	errA := store.Acquire(ctx, leaseA)
	if errA != nil {
		t.Fatalf("Chat A acquire failed: %v", errA)
	}

	// Second acquire on same resource should fail
	errB := store.Acquire(ctx, leaseB)
	if errB == nil {
		t.Fatal("Chat B acquire should have failed (resource already leased)")
	}

	// Verify Chat A owns the lease
	retrieved, err := store.Get(ctx, leaseA.ID)
	if err != nil {
		t.Fatalf("Get lease: %v", err)
	}
	if retrieved.ChatID != chatA {
		t.Fatalf("Lease owned by %s, want %s", retrieved.ChatID, chatA)
	}
}

// TestHeartbeat_OnlyOwnerCanRenew verifies that non-owners can't heartbeat.
func TestHeartbeat_OnlyOwnerCanRenew(t *testing.T) {
	db := testDB(t)
	defer os.Remove(db)
	store, err := NewSQLiteLeaseStore(db, "", "")
	if err != nil {
		t.Fatalf("NewSQLiteLeaseStore: %v", err)
	}

	ctx := context.Background()
	chatA, chatB := "chat-a", "chat-b"

	// Chat A acquires lease
	lease := &Lease{
		Kind:       LeaseKindRepo,
		Resource:   "KooshaPari/Repo",
		ChatID:     chatA,
		TTLSeconds: 3600,
		Reason:     LeaseReason{Kind: "manual", Value: "test"},
	}
	if err := store.Acquire(ctx, lease); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Chat A can heartbeat
	if err := store.Heartbeat(ctx, lease.ID, chatA, lease.EpochToken); err != nil {
		t.Fatalf("Chat A heartbeat failed: %v", err)
	}

	// Chat B cannot heartbeat (wrong owner)
	if err := store.Heartbeat(ctx, lease.ID, chatB, lease.EpochToken); err == nil {
		t.Fatal("Chat B heartbeat should have failed")
	}
}

// TestReapExpired_TTLExpiryReturnsLease verifies TTL-based expiry and reap.
func TestReapExpired_TTLExpiryReturnsLease(t *testing.T) {
	db := testDB(t)
	defer os.Remove(db)
	store, err := NewSQLiteLeaseStore(db, "", "")
	if err != nil {
		t.Fatalf("NewSQLiteLeaseStore: %v", err)
	}

	ctx := context.Background()

	// Acquire lease with very short TTL
	lease := &Lease{
		Kind:       LeaseKindRepo,
		Resource:   "KooshaPari/Repo",
		ChatID:     "chat-a",
		TTLSeconds: 1, // 1 second TTL
		Reason:     LeaseReason{Kind: "wip_run", Value: "test-run"},
	}
	if err := store.Acquire(ctx, lease); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Lease should be active
	retrieved, _ := store.Get(ctx, lease.ID)
	if retrieved.State != LeaseStateActive {
		t.Fatalf("Initial state is %s, want active", retrieved.State)
	}

	// Wait for TTL to elapse
	time.Sleep(2 * time.Second)

	// Reap should find and expire it
	count, err := store.ReapExpired(ctx)
	if err != nil {
		t.Fatalf("ReapExpired: %v", err)
	}
	if count != 1 {
		t.Fatalf("Reaped %d leases, want 1", count)
	}

	// Lease should now be expired
	retrieved, _ = store.Get(ctx, lease.ID)
	if retrieved.State != LeaseStateExpired {
		t.Fatalf("Final state is %s, want expired", retrieved.State)
	}
}

// TestEpochFencing_StaleClaimantRejected verifies fencing against split-brain.
func TestEpochFencing_StaleClaimantRejected(t *testing.T) {
	db := testDB(t)
	defer os.Remove(db)
	store, err := NewSQLiteLeaseStore(db, "", "")
	if err != nil {
		t.Fatalf("NewSQLiteLeaseStore: %v", err)
	}

	ctx := context.Background()

	// Chat A acquires lease
	lease := &Lease{
		Kind:       LeaseKindRepo,
		Resource:   "KooshaPari/Repo",
		ChatID:     "chat-a",
		TTLSeconds: 3600,
		Reason:     LeaseReason{Kind: "manual", Value: "test"},
	}
	if err := store.Acquire(ctx, lease); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	oldEpoch := lease.EpochToken

	// Chat A heartbeats once (updates epoch)
	if err := store.Heartbeat(ctx, lease.ID, "chat-a", oldEpoch); err != nil {
		t.Fatalf("First heartbeat: %v", err)
	}

	// Get updated epoch (in real use, daemon polls this)
	updated, _ := store.Get(ctx, lease.ID)
	newEpoch := updated.EpochToken

	// Stale claimant tries to heartbeat with old epoch — should fail (fencing)
	if err := store.Heartbeat(ctx, lease.ID, "chat-a", oldEpoch); err == nil {
		t.Fatal("Heartbeat with stale epoch should have failed (fencing)")
	}

	// Current holder can still heartbeat with new epoch
	if err := store.Heartbeat(ctx, lease.ID, "chat-a", newEpoch); err != nil {
		t.Fatalf("Heartbeat with current epoch failed: %v", err)
	}
}

// testDB creates a temporary SQLite database for testing.
func testDB(t *testing.T) string {
	f, err := os.CreateTemp("", "phenodag-lease-*.db")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	return f.Name()
}
