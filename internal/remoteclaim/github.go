package remoteclaim

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// GHRunner executes gh CLI commands (injectable for tests).
type GHRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

type ghCLI struct{}

func (ghCLI) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, msg)
		}
		return nil, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// GitHubConfig holds coordination issue settings.
type GitHubConfig struct {
	Owner       string
	Repo        string
	IssueNumber int
}

// GitHubTransport publishes claim state to a GitHub tracking issue via gh CLI.
type GitHubTransport struct {
	cfg    GitHubConfig
	store  *SQLiteStore
	gh     GHRunner
}

// NewGitHubTransport creates a GitHub-backed transport. Local SQLite remains
// source of truth; GitHub labels+comments provide cross-host coordination.
func NewGitHubTransport(cfg GitHubConfig, store *SQLiteStore, gh GHRunner) *GitHubTransport {
	if gh == nil {
		gh = ghCLI{}
	}
	return &GitHubTransport{cfg: cfg, store: store, gh: gh}
}

func (t *GitHubTransport) repoSlug() string {
	return t.cfg.Owner + "/" + t.cfg.Repo
}

func (t *GitHubTransport) Claim(ctx context.Context, c Claim) (*Claim, error) {
	if t.cfg.IssueNumber == 0 {
		return nil, ErrNoTracking
	}
	label := LabelName(c.Kind, c.Repo, c.Branch, c.Worktree)
	issued, err := t.store.Claim(ctx, c)
	if err != nil {
		return nil, err
	}
	if err := t.addLabel(ctx, label); err != nil {
		if isLabelConflict(err) {
			holder, readErr := t.readLabelHolder(ctx, label)
			if readErr == nil && holder != "" && holder != issued.AgentID {
				_ = t.store.ForceRelease(ctx, issued.ID, "github_conflict")
				return nil, ErrConflict
			}
		} else {
			return issued, err
		}
	}
	ev := t.event("claim", issued)
	if err := t.postComment(ctx, ev); err != nil {
		return issued, err
	}
	return issued, nil
}

func (t *GitHubTransport) Heartbeat(ctx context.Context, claimID, agentID string, seenEpoch int64) error {
	if t.cfg.IssueNumber == 0 {
		return ErrNoTracking
	}
	c, err := t.store.Get(ctx, claimID)
	if err != nil {
		return err
	}
	if c == nil {
		return ErrNotFound
	}
	remoteEpoch, err := t.latestEpoch(ctx, claimID)
	if err == nil && remoteEpoch > seenEpoch {
		return ErrStaleEpoch
	}
	if err := t.store.Heartbeat(ctx, claimID, agentID, seenEpoch); err != nil {
		return err
	}
	ev := Event{
		V: 1, Event: "heartbeat", ClaimID: claimID, Agent: agentID,
		Epoch: seenEpoch, At: time.Now().UTC().Format(time.RFC3339),
	}
	return t.postComment(ctx, ev)
}

func (t *GitHubTransport) Release(ctx context.Context, claimID, agentID string, seenEpoch int64) error {
	if t.cfg.IssueNumber == 0 {
		return ErrNoTracking
	}
	c, err := t.store.Get(ctx, claimID)
	if err != nil {
		return err
	}
	if c == nil {
		return ErrNotFound
	}
	if err := t.store.Release(ctx, claimID, agentID, seenEpoch); err != nil {
		return err
	}
	label := LabelName(c.Kind, c.Repo, c.Branch, c.Worktree)
	_ = t.removeLabel(ctx, label)
	ev := Event{
		V: 1, Event: "release", ClaimID: claimID, Agent: agentID,
		Epoch: seenEpoch + 1, At: time.Now().UTC().Format(time.RFC3339),
	}
	return t.postComment(ctx, ev)
}

func (t *GitHubTransport) Transfer(ctx context.Context, fromID, toID, toAgent string) error {
	if t.cfg.IssueNumber == 0 {
		return ErrNoTracking
	}
	old, err := t.store.Get(ctx, fromID)
	if err != nil {
		return err
	}
	if old == nil {
		return ErrNotFound
	}
	newClaim, err := t.store.Transfer(ctx, fromID, toID, toAgent)
	if err != nil {
		return err
	}
	oldLabel := LabelName(old.Kind, old.Repo, old.Branch, old.Worktree)
	_ = t.removeLabel(ctx, oldLabel)
	_ = t.addLabel(ctx, LabelName(newClaim.Kind, newClaim.Repo, newClaim.Branch, newClaim.Worktree))
	ev := Event{
		V: 1, Event: "transfer", ClaimID: fromID, ToClaim: newClaim.ID, ToAgent: toAgent,
		Epoch: newClaim.Epoch, At: time.Now().UTC().Format(time.RFC3339),
	}
	return t.postComment(ctx, ev)
}

func (t *GitHubTransport) ReapExpired(ctx context.Context, now time.Time) ([]Claim, error) {
	if t.cfg.IssueNumber == 0 {
		return nil, ErrNoTracking
	}
	reaped, err := t.store.ReapExpired(ctx, now)
	if err != nil {
		return nil, err
	}
	for _, c := range reaped {
		label := LabelName(c.Kind, c.Repo, c.Branch, c.Worktree)
		_ = t.removeLabel(ctx, label)
		ev := Event{
			V: 1, Event: "reap", ClaimID: c.ID, Reason: "ttl_expired",
			Resource: c.Resource, Kind: string(c.Kind), Epoch: c.Epoch + 1,
			At: now.UTC().Format(time.RFC3339),
		}
		_ = t.postComment(ctx, ev)
	}
	return reaped, nil
}

func (t *GitHubTransport) List(ctx context.Context) ([]Claim, error) {
	local, err := t.store.Active(ctx)
	if err != nil {
		return nil, err
	}
	if t.cfg.IssueNumber == 0 {
		return local, nil
	}
	// Merge GitHub comment epochs for drift detection.
	for i := range local {
		if epoch, err := t.latestEpoch(ctx, local[i].ID); err == nil && epoch > local[i].Epoch {
			local[i].Epoch = epoch
		}
	}
	return local, nil
}

func (t *GitHubTransport) addLabel(ctx context.Context, label string) error {
	_, err := t.gh.Run(ctx,
		"api", "-X", "POST",
		fmt.Sprintf("repos/%s/%s/issues/%d/labels", t.cfg.Owner, t.cfg.Repo, t.cfg.IssueNumber),
		"-f", "labels[]="+label,
	)
	return err
}

func (t *GitHubTransport) removeLabel(ctx context.Context, label string) error {
	_, err := t.gh.Run(ctx,
		"api", "-X", "DELETE",
		fmt.Sprintf("repos/%s/%s/issues/%d/labels/%s", t.cfg.Owner, t.cfg.Repo, t.cfg.IssueNumber, label),
	)
	return err
}

func (t *GitHubTransport) postComment(ctx context.Context, ev Event) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = t.gh.Run(ctx,
		"api", "-X", "POST",
		fmt.Sprintf("repos/%s/%s/issues/%d/comments", t.cfg.Owner, t.cfg.Repo, t.cfg.IssueNumber),
		"-f", "body="+string(body),
	)
	return err
}

func (t *GitHubTransport) latestEpoch(ctx context.Context, claimID string) (int64, error) {
	out, err := t.gh.Run(ctx,
		"api",
		fmt.Sprintf("repos/%s/%s/issues/%d/comments?per_page=100", t.cfg.Owner, t.cfg.Repo, t.cfg.IssueNumber),
	)
	if err != nil {
		return 0, err
	}
	var comments []struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(out, &comments); err != nil {
		return 0, err
	}
	var epoch int64
	for _, c := range comments {
		var ev Event
		if json.Unmarshal([]byte(c.Body), &ev) != nil {
			continue
		}
		if ev.ClaimID == claimID && ev.Epoch > epoch {
			epoch = ev.Epoch
		}
	}
	return epoch, nil
}

func (t *GitHubTransport) readLabelHolder(ctx context.Context, label string) (string, error) {
	out, err := t.gh.Run(ctx,
		"api",
		fmt.Sprintf("repos/%s/%s/issues/%d", t.cfg.Owner, t.cfg.Repo, t.cfg.IssueNumber),
	)
	if err != nil {
		return "", err
	}
	var issue struct {
		Labels []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(out, &issue); err != nil {
		return "", err
	}
	for _, l := range issue.Labels {
		if l.Name == label {
			return l.Description, nil
		}
	}
	return "", nil
}

func (t *GitHubTransport) event(name string, c *Claim) Event {
	return Event{
		V: 1, Event: name, ClaimID: c.ID, Agent: c.AgentID,
		Resource: c.Resource, Kind: string(c.Kind), Epoch: c.Epoch, TTL: c.TTLSeconds,
		At: time.Now().UTC().Format(time.RFC3339),
	}
}

func isLabelConflict(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "422") || strings.Contains(s, "already exists")
}

// ResolveGitHubConfig loads coordination repo/issue from env or flags.
func ResolveGitHubConfig(coordRepo string, issueNum int) (GitHubConfig, error) {
	if coordRepo == "" {
		coordRepo = os.Getenv("DAGCTL_REMOTE_CLAIM_REPO")
	}
	if issueNum == 0 {
		if v := os.Getenv("DAGCTL_REMOTE_CLAIM_ISSUE"); v != "" {
			fmt.Sscanf(v, "%d", &issueNum)
		}
	}
	if coordRepo == "" {
		return GitHubConfig{}, ErrNoTracking
	}
	owner, name, err := ParseOwnerRepo(coordRepo)
	if err != nil {
		return GitHubConfig{}, err
	}
	if issueNum == 0 {
		return GitHubConfig{}, fmt.Errorf("%w: set --issue or DAGCTL_REMOTE_CLAIM_ISSUE", ErrNoTracking)
	}
	return GitHubConfig{Owner: owner, Repo: name, IssueNumber: issueNum}, nil
}

// NewTransport selects local or GitHub transport.
func NewTransport(mode string, store *SQLiteStore, coordRepo string, issueNum int) (Transport, error) {
	switch mode {
	case "", "local":
		return NewLocalTransport(store), nil
	case "github":
		cfg, err := ResolveGitHubConfig(coordRepo, issueNum)
		if err != nil {
			return nil, err
		}
		return NewGitHubTransport(cfg, store, nil), nil
	default:
		return nil, fmt.Errorf("unknown transport %q (use local or github)", mode)
	}
}
