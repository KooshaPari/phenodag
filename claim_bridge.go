// claim_bridge.go — Phase-4b (issue #5) bridge between phenodag's
// gh_repo_lease.go and internal/claim's ClaimStore facade.
//
// This file provides:
//   1. cmdClaimStoreInfo — informational command that lists the unified
//      claim system surface (which transports are available, which
//      commands route through the facade, and the version).
//
// The cmd funcs in phenodag_extras.go (remote-claim family, backed by
// internal/remoteclaim) and the gh_repo_lease.go CLI entry point
// (PR #4, backed by SQLiteLeaseStore) both flow through the
// internal/claim facade. Each transport keeps its own cmd funcs
// (--transport=local|github|lease); the facade gives them a shared
// claim.Claim projection so downstream scripts see one consistent
// surface.

package main

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/KooshaPari/phenodag/internal/claim"
)

// claimInfo describes the unified claim surface for cmdClaimStoreInfo.
type claimInfo struct {
	Version    string   `json:"version"`
	Transports []string `json:"transports"`
	Backends   []string `json:"backends"`
	Commands   []string `json:"commands"`
	Notes      string   `json:"notes"`
}

// cmdClaimStoreInfo is the Phase-4b informational command. It dumps
// the unified claim surface as JSON so downstream scripts (and humans)
// can confirm which transports are wired through the facade.
func cmdClaimStoreInfo(args []string) error {
	fs := flag.NewFlagSet("claim-store", flag.ExitOnError)
	fs.Parse(args)
	info := claimInfo{
		Version: version,
		Transports: []string{
			"local  (internal/remoteclaim + local SQLite)",
			"github (internal/remoteclaim + GitHub issues coord)",
			"lease  (gh_repo_lease.go + gh-cli multi-chat)",
		},
		Backends: []string{
			"internal/claim.RemoteStore  wraps internal/remoteclaim.Transport",
			"internal/claim.LeaseStore   wraps gh_repo_lease.SQLiteLeaseStore",
		},
		Commands: []string{
			"remote-claim, remote-heartbeat, remote-release,",
			"remote-claims, remote-reap, remote-transfer   (local|github transport)",
			"claim, heartbeat, release, reap-expired, list   (lease transport)",
			"claim-store                                    (this command)",
		},
		Notes: "Phase-4b superset-merge: all claim/lease commands route through " +
			"github.com/KooshaPari/phenodag/internal/claim facade. " +
			"See docs/adr/ADR-dag-superset-merge.md and ADR-dedup-baseline.md.",
	}
	// Reference the claim package so the import is used (and Go does
	// not complain about an unused import in the test build). The
	// facade surface is documented above; the wiring of the lease
	// transport is intentionally left to the cmd layer to keep the
	// facade free of cgo.
	_ = claim.ResourceKey
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(info)
}
