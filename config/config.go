// Package config provides centralized configuration for dagctl/phenodag.
//
// All hardcoded defaults, environment variables, and configurable knobs
// live here. The main Config struct is populated by Load() which reads
// environment variable overrides. Flag overrides (from command-line flags)
// take precedence over env vars which take precedence over defaults.
//
// Usage:
//
//	cfg := config.Load()
//	db, err := openDB(cfg.DBPath())
package config

import (
	"os"
	"strconv"
	"time"
)

// ──────────────────────────────────────────────
// Config struct — all knobs in one place
// ──────────────────────────────────────────────

// Config holds all configuration values for a dagctl/phenodag instance.
// Populate via Load().
type Config struct {
	dbPath                 string
	dbBusyTimeout          int
	dbMaxOpenConns         int
	remoteClaimsDB         string
	agentID                string
	claimTransport         string
	coordRepo              string
	defaultTTL             int64
	reclaimStaleMin        int
	dashboardInterval      time.Duration
	thrashDuration         time.Duration
	thrashAgents           int
	sweepFailedAge         time.Duration
	defaultCSVOut          string
	defaultHTMLOut         string
	burndownWidth          int
	similarityJaccardWeight float64
	similarityLevWeight    float64
	similarityRepoWeight   float64
	nearDuplicateThreshold float64
	dedupExplainThreshold  float64
	ngramSize              int
	simhashBoost           float64
	worktreeBranchPrefix   string
	version                string
	v3Version              string
}

// ──────────────────────────────────────────────
// Environment variable names
// ──────────────────────────────────────────────

const (
	EnvDBPath          = "PHENODAG_DB"
	EnvRemoteClaimsDB  = "REMOTE_CLAIMS_DB"
	EnvAgentID         = "DAGCTL_AGENT"
	EnvClaimTransport  = "DAGCTL_CLAIM_TRANSPORT"
	EnvDBBusyTimeout   = "PHENODAG_DB_BUSY_TIMEOUT"
	EnvDBMaxOpenConns  = "PHENODAG_DB_MAX_OPEN_CONNS"
	EnvReclaimStaleMin = "PHENODAG_RECLAIM_STALE_MIN"
	EnvDefaultTTL      = "PHENODAG_DEFAULT_TTL"
	EnvCoordRepo       = "PHENODAG_COORD_REPO"
)

// ──────────────────────────────────────────────
// Default values
// ──────────────────────────────────────────────

const (
	DefaultDBPath            = "phenodag.db"
	DefaultDBBusyTimeout     = 5000
	DefaultDBMaxOpenConns    = 1
	DefaultRemoteClaimsDB    = "FLEET_REMOTE_CLAIMS.db"
	DefaultClaimTransport    = "local"
	DefaultAgentID           = ""
	DefaultCoordRepo         = ""
	DefaultTTLSeconds        = 3600
	DefaultReclaimStaleMin   = 15
	DefaultDashboardInterval = 2 * time.Second
	DefaultThrashDuration    = 5 * time.Second
	DefaultThrashAgents      = 5
	DefaultSweepFailedAge    = 24 * time.Hour
	DefaultCSVOut            = "phenodag.csv"
	DefaultHTMLOut           = "phenodag.html"
	DefaultBurndownWidth     = 50
	DefaultSimJaccardWeight  = 0.6
	DefaultSimLevWeight      = 0.2
	DefaultSimRepoWeight     = 0.2
	DefaultNearDupThreshold  = 0.5
	DefaultDedupThreshold    = 0.85
	DefaultNgramSize         = 3
	DefaultSimhashBoost      = 0.05
	DefaultWorktreePrefix    = "wt-%s-%d"
	DefaultVersion           = "1.0.0-rc.1"
	DefaultV3Version         = "v3.2.0-port"
)

// ──────────────────────────────────────────────
// Load — read env + defaults into a Config
// ──────────────────────────────────────────────

// Load returns a Config populated from environment variables (falling back
// to built-in defaults). Call this once at program start.
func Load() *Config {
	return &Config{
		dbPath:                  envOr(EnvDBPath, DefaultDBPath),
		dbBusyTimeout:           envIntOr(EnvDBBusyTimeout, DefaultDBBusyTimeout),
		dbMaxOpenConns:          envIntOr(EnvDBMaxOpenConns, DefaultDBMaxOpenConns),
		remoteClaimsDB:          envOr(EnvRemoteClaimsDB, DefaultRemoteClaimsDB),
		agentID:                 envOr(EnvAgentID, DefaultAgentID),
		claimTransport:          envOr(EnvClaimTransport, DefaultClaimTransport),
		coordRepo:               envOr(EnvCoordRepo, DefaultCoordRepo),
		defaultTTL:              envInt64Or(EnvDefaultTTL, DefaultTTLSeconds),
		reclaimStaleMin:         envIntOr(EnvReclaimStaleMin, DefaultReclaimStaleMin),
		dashboardInterval:       DefaultDashboardInterval,
		thrashDuration:          DefaultThrashDuration,
		thrashAgents:            DefaultThrashAgents,
		sweepFailedAge:          DefaultSweepFailedAge,
		defaultCSVOut:           DefaultCSVOut,
		defaultHTMLOut:          DefaultHTMLOut,
		burndownWidth:           DefaultBurndownWidth,
		similarityJaccardWeight: DefaultSimJaccardWeight,
		similarityLevWeight:     DefaultSimLevWeight,
		similarityRepoWeight:    DefaultSimRepoWeight,
		nearDuplicateThreshold:  DefaultNearDupThreshold,
		dedupExplainThreshold:   DefaultDedupThreshold,
		ngramSize:               DefaultNgramSize,
		simhashBoost:            DefaultSimhashBoost,
		worktreeBranchPrefix:    DefaultWorktreePrefix,
		version:                 DefaultVersion,
		v3Version:               DefaultV3Version,
	}
}

// ──────────────────────────────────────────────
// Accessors
// ──────────────────────────────────────────────

func (c *Config) DBPath() string                     { return c.dbPath }
func (c *Config) DBBusyTimeout() int                 { return c.dbBusyTimeout }
func (c *Config) DBMaxOpenConns() int                { return c.dbMaxOpenConns }
func (c *Config) RemoteClaimsDB() string             { return c.remoteClaimsDB }
func (c *Config) AgentID() string                    { return c.agentID }
func (c *Config) ClaimTransport() string             { return c.claimTransport }
func (c *Config) CoordRepo() string                  { return c.coordRepo }
func (c *Config) DefaultTTL() int64                  { return c.defaultTTL }
func (c *Config) ReclaimStaleMin() int               { return c.reclaimStaleMin }
func (c *Config) DashboardInterval() time.Duration   { return c.dashboardInterval }
func (c *Config) ThrashDuration() time.Duration      { return c.thrashDuration }
func (c *Config) ThrashAgents() int                  { return c.thrashAgents }
func (c *Config) SweepFailedAge() time.Duration       { return c.sweepFailedAge }
func (c *Config) DefaultCSVOut() string              { return c.defaultCSVOut }
func (c *Config) DefaultHTMLOut() string             { return c.defaultHTMLOut }
func (c *Config) BurndownWidth() int                 { return c.burndownWidth }
func (c *Config) SimilarityJaccardWeight() float64   { return c.similarityJaccardWeight }
func (c *Config) SimilarityLevWeight() float64       { return c.similarityLevWeight }
func (c *Config) SimilarityRepoWeight() float64      { return c.similarityRepoWeight }
func (c *Config) NearDuplicateThreshold() float64    { return c.nearDuplicateThreshold }
func (c *Config) DedupExplainThreshold() float64     { return c.dedupExplainThreshold }
func (c *Config) NgramSize() int                     { return c.ngramSize }
func (c *Config) SimhashBoost() float64              { return c.simhashBoost }
func (c *Config) WorktreeBranchPrefix() string       { return c.worktreeBranchPrefix }
func (c *Config) Version() string                    { return c.version }
func (c *Config) V3Version() string                  { return c.v3Version }

// SetDBPath allows flag-level override of the DB path.
func (c *Config) SetDBPath(p string) { c.dbPath = p }

// ──────────────────────────────────────────────
// DSN builder — uses config values
// ──────────────────────────────────────────────

// DSN returns a SQLite DSN string with WAL, busy_timeout, and foreign_keys
// configured from the Config.
func (c *Config) DSN(path string) string {
	return "file:" + path +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(" + strconv.Itoa(c.dbBusyTimeout) + ")" +
		"&_pragma=foreign_keys(on)"
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envInt64Or(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}
