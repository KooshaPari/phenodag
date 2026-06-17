# ADR: GitHub-Aware Multi-Chat Repo Lease/Claim System

**Status:** Accepted (Phase 4 signature feature)  
**Date:** 2026-06-17  
**Author:** L1-Charlie-DAG  
**Context:** FR-PHEN-044 (repo multi-chat coordination)

## Problem

Multiple agent chats can mutate the same remote repository in parallel, causing:
- Silent conflicts (PR #754 + bd318bef both touching AgilePlus main concurrently)
- Forced superset-merge resolution work
- Lost audit trail of who changed what when

Example: Chat-A opens a superset-merge PR on phenodag while Chat-B tries to port dagctl v3 engine into the same files → merge conflict + manual resolution overhead.

## Solution: Atomic, GitHub-Coordinated Lease with TTL + Fencing

Adopt a proven **claim primitive** (from AgilePlus traceability/triage layer) adapted for cross-chat GitHub coordination:

1. **Resource Type**: `Lease(LeaseKind, resource, state, chat_id, ttl_seconds, epoch_token)`
   - `Kind`: repo | branch | worktree | subrepo
   - `State`: active | draining | expired
   - `EpochToken`: Monotonic clock (Unix timestamp) for fencing against stale claimants

2. **Core Operations**:
   - **Acquire**: Chat claims `KooshaPari/Repo` atomically; fails if already held
   - **Heartbeat**: Periodic renewal; rejects if epoch mismatch (prevents split-brain)
   - **Release**: Voluntary return; TTL-based auto-reap for crashed chats
   - **Transfer**: Failover from dead chat to live one (fencing prevents race)
   - **ReapExpired**: Daemon reclaims expired leases (TTL > now)

3. **Persistence & Coordination**:
   - **Local**: SQLite + POSIX flock (single-machine, CLI use)
   - **Multi-chat**: GitHub coordination repo (committed audit log + gh-cli bridge for visibility)
   - **Fencing**: Epoch tokens prevent split-brain if a chat crashes mid-lease

4. **Guarantees**:
   - **Mutual Exclusion**: Only one chat can hold active lease per resource
   - **Eventual Release**: Crashed chat's lease expires and is reaped
   - **Cross-Chat Audit**: Coordination repo logs all acquire/heartbeat/release events
   - **No Ghost Claims**: Epoch fencing rejects stale heartbeats from restarted/confused chats

## Design Reuse

- **Claim Semantics**: Identical to AgilePlus `crates/agileplus-triage/src/claim.rs` (ClaimKind, ClaimState, ClaimReason, TTL, heartbeat, reap)
- **Remote Coordination**: Extends dagctl `internal/remoteclaim/` (SQLite, flock, GitHub transport layer)
- **xDD Tests**: Concurrent acquire, heartbeat fencing, TTL reap, transfer failover

## Implementation

File: `gh_repo_lease.go` (438 lines)
Tests: `gh_repo_lease_test.go` (xDD suite: 4 scenarios)

**Key Types**:
```go
type Lease struct {
	ID            string    // UUID
	Kind          LeaseKind // repo | branch | worktree | subrepo
	Resource      string    // owner/repo[:branch][:worktree]
	State         LeaseState // active | draining | expired
	ChatID        string    // Claiming chat/agent ID
	EpochToken    int64     // Fencing clock (Unix sec)
	TTLSeconds    int64     // Default 3600
	AcquiredAt    time.Time
	LastHeartbeat time.Time
	Reason        LeaseReason // Structured claim reason
}

type LeaseStore interface {
	Acquire(ctx, lease) error       // Atomic claim
	Heartbeat(ctx, id, chatID, epoch) error // Renewal + fencing
	Release(ctx, id, chatID) error  // Voluntary return
	ReapExpired(ctx) (int, error)   // TTL expiry → expired state
	Transfer(ctx, id, oldChat, newChat, epoch) error // Failover
	Get(ctx, id) (*Lease, error)    // Retrieve
	ListActive(ctx) ([]*Lease, error) // All held claims
}
```

**CLI Commands**:
```
phenodag claim KooshaPari/Repo --agent <chat-id> --ttl 3600 --reason "pr/phase4"
phenodag heartbeat --claim-id <id> --agent <chat-id> --epoch <token>
phenodag release --claim-id <id> --agent <chat-id>
phenodag reap-expired [--dry-run]
```

## Trade-offs

| Decision | Rationale |
|----------|-----------|
| SQLite for persistence | Proven in AgilePlus triage; ACID guarantees; portable |
| Epoch fencing (not consensus) | Simpler than Raft/Paxos; sufficient for TTL-bounded leases |
| GitHub coordination repo | Audit trail visible to all chats; no central coordinator needed |
| TTL + daemon reap | Handles crashed chats automatically; no manual intervention |
| Structured LeaseReason (not free-form) | Better for audit queries; matches AgilePlus pattern |

## Testing

xDD (BDD + DD) test scenarios:
1. **ConcurrentClaim_OnlyOneWins**: Two chats race → one succeeds, one fails
2. **Heartbeat_OnlyOwnerCanRenew**: Non-owner heartbeats rejected
3. **ReapExpired_TTLExpiryReturnsLease**: TTL elapses, reap returns lease
4. **EpochFencing_StaleClaimantRejected**: Old epoch rejected (prevents split-brain)
5. **Transfer_Failover**: Lease moves from crashed chat to live one

## Phase-4 Impact

This feature is **shipped as part of the phenodag superset-merge**:
- **Phase 3 → 4 boundary**: Unifies dagctl + phenodag into a single coordinated repo ecosystem
- **Multi-chat safety**: Enables 10+ parallel chats to work on different repos without conflicts
- **Audit trail**: Every lease event logged for compliance/debugging
- **Template for AgilePlus**: The lease primitive can extend to branch/worktree/subproject claims later

## References

- **AgilePlus claim engine**: `crates/agileplus-triage/src/claim.rs`
- **dagctl remoteclaim**: `internal/remoteclaim/{flock,github,sqlite,store}.rs`
- **FR-PHEN-044**: Repo multi-chat coordination primitive
- **Issue #754**: Example conflict that this prevents
