# Changelog

All notable changes to phenodag will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v3.3.1] - 2026-06-18

### Added
- `VERSION` file pinning the v3.3.1 final tag (absorbed from `KooshaPari/dagctl`)
- Migration entry: this release completes the dagctl → phenodag absorption per kilo audit #144
  and `ADR-dag-superset-merge.md` (Accepted). The single-binary Go CLI surface from `dagctl/`
  (v3.0.0 → v3.3.1) has been merged into phenodag's `phenodag.go` + extensions.

### Notes
- `KooshaPari/dagctl` is **archived** (read-only marker; manual `gh repo delete` required for
  actual removal — the active `gh` token only has `'gist', 'read:org', 'repo', 'workflow'`
  scopes, no `delete_repo`).
- Phenodag file renaming (`dagctl_*.go` → `phenodag_*.go`) and content merge into
  `phenodag.go` (65,994 B) is already complete from earlier waves.
- Tag `v1.0.0-rc.1` exists on `main` from a prior release window; `v3.3.1` aligns the
  versioning with the absorbed dagctl line so downstream `dagctl-merge-status.md` parity is
  maintained.

## [v0.3.0] - 2026-06-13

### Added
- 15 CLI commands; v0.3.0 plan complete.

[Unreleased]: https://github.com/KooshaPari/phenodag/compare/v0.3.0...HEAD
[v3.3.1]: https://github.com/KooshaPari/phenodag/releases/tag/v3.3.1
[v0.3.0]: https://github.com/KooshaPari/phenodag/releases/tag/v0.3.0
## [Unreleased]

### Changed

- **2026-07-05: thin redirector placed.** phenodag features are being
  absorbed into Tracera (DAG/queue/atomic-claim/lease) and AgilePlus
  (PM/cockpit). This repo will be archived after 1 release cycle.
  See README.md and the polyrepo portfolio strategy session at
  `docs/sessions/2026-07-05-polyrepo-portfolio-strategy/`.
