package remoteclaim

import (
	"errors"
	"time"
)

// State mirrors AgilePlus ClaimState.
type State string

const (
	StateActive   State = "active"
	StateDraining State = "draining"
	StateExpired  State = "expired"
)

// Kind is the resource type being claimed.
type Kind string

const (
	KindRepo       Kind = "repo"
	KindBranch     Kind = "branch"
	KindWorktree   Kind = "worktree"
	KindSubproject Kind = "subproject"
)

// ReasonKind mirrors AgilePlus ClaimReason variants.
type ReasonKind string

const (
	ReasonTaskRef    ReasonKind = "task_ref"
	ReasonBranch     ReasonKind = "branch"
	ReasonSubproject ReasonKind = "subproject"
	ReasonWipRun     ReasonKind = "wip_run"
	ReasonManual     ReasonKind = "manual"
)

// Reason is a structured claim reason.
type Reason struct {
	Kind  ReasonKind
	Value string
}

// Claim is a remote resource claim with TTL, heartbeat, and fencing epoch.
type Claim struct {
	ID            string    `json:"claim_id"`
	AgentID       string    `json:"agent_id"`
	Resource      string    `json:"resource"`
	Kind          Kind      `json:"kind"`
	Repo          string    `json:"repo"`
	Branch        string    `json:"branch,omitempty"`
	Worktree      string    `json:"worktree,omitempty"`
	Status        State     `json:"status"`
	Reason        Reason    `json:"reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	TTLSeconds    int64     `json:"ttl_seconds"`
	Epoch         int64     `json:"epoch"`
	TaskID        string    `json:"task_id,omitempty"`
}

func (c Claim) IsExpired(now time.Time) bool {
	return now.Sub(c.LastHeartbeat).Milliseconds() > c.TTLSeconds*1000
}

// Event is the JSON envelope written to GitHub issue comments.
type Event struct {
	V        int    `json:"v"`
	Event    string `json:"event"`
	ClaimID  string `json:"claim_id"`
	Agent    string `json:"agent,omitempty"`
	ToClaim  string `json:"to_claim_id,omitempty"`
	ToAgent  string `json:"to_agent,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Resource string `json:"resource,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Epoch    int64  `json:"epoch"`
	TTL      int64  `json:"ttl,omitempty"`
	At       string `json:"at"`
}

var (
	ErrConflict   = errors.New("claim conflict: resource already held")
	ErrNotFound   = errors.New("claim not found")
	ErrWrongOwner = errors.New("wrong claim owner")
	ErrWrongState = errors.New("wrong claim state for operation")
	ErrStaleEpoch = errors.New("stale epoch: fencing rejected write")
	ErrNoTracking = errors.New("no tracking issue configured")
)

// ParseOwnerRepo splits "owner/name" into owner and name.
func ParseOwnerRepo(slug string) (owner, name string, err error) {
	parts := splitOwnerRepo(slug)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("expected owner/repo slug")
	}
	return parts[0], parts[1], nil
}

func splitOwnerRepo(slug string) []string {
	for i := 0; i < len(slug); i++ {
		if slug[i] == '/' {
			return []string{slug[:i], slug[i+1:]}
		}
	}
	return []string{slug}
}

// ResourceKey builds canonical resource string for a repo claim.
func ResourceKey(kind Kind, repo, branch, worktree string) string {
	switch kind {
	case KindRepo:
		return "repo:" + repo
	case KindBranch:
		return "branch:" + repo + ":" + branch
	case KindWorktree:
		return "worktree:" + repo + ":" + branch + ":" + worktree
	case KindSubproject:
		return "subproject:" + repo
	default:
		return string(kind) + ":" + repo
	}
}

// LabelName returns the GitHub label for a claim resource.
func LabelName(kind Kind, repo, branch, worktree string) string {
	switch kind {
	case KindRepo:
		return "claim:repo/" + repo
	case KindBranch:
		return "claim:branch/" + repo + "/" + branch
	case KindWorktree:
		return "claim:worktree/" + repo + "/" + branch + "/" + worktree
	default:
		return "claim:" + string(kind) + "/" + repo
	}
}
