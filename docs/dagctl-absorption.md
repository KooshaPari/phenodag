# phenodag — dagctl Absorption Migration (v3.3.1)

This document records the completed migration of [`KooshaPari/dagctl`](https://github.com/KooshaPari/dagctl) → [`KooshaPari/phenodag`](https://github.com/KooshaPari/phenodag) per kilo audit #144 (verdict: `DELETE_AFTER_PATCHES` → `ARCHIVED`) and the in-repo `ADR-dag-superset-merge.md` (Accepted).

## Status: ✅ ABSORPTION COMPLETE (2026-06-18)

| Phase | Status | Evidence |
|---|---|---|
| 1. File rename (`dagctl_*.go` → `phenodag_*.go`) | DONE (prior waves) | `git log --diff-filter=R` on main |
| 2. Content merge into `phenodag.go` (65,994 B) | DONE (prior waves) | file size reflects merged v3 surface |
| 3. v3.3.1 final tag patch applied (VERSION file) | DONE this PR | `VERSION` → `v3.3.1` |
| 4. `CHANGELOG.md` v3.3.1 entry | DONE this PR | this file |
| 5. `KooshaPari/dagctl` archive (read-only marker) | DONE (prior session, 2026-06-17 22:44) | `gh api repos/KooshaPari/dagctl` → `archived: true` |
| 6. `KooshaPari/dagctl` delete | **BLOCKED** — `gh` token lacks `delete_repo` scope | see "Manual delete commands" below |

## Files merged

| dagctl source | phenodag target | Status |
|---|---|---|
| `dagctl.go` (39,397 B) | merged into `phenodag.go` (65,994 B) | DONE |
| `dagctl_v3_seed.go` (11,317 B) | merged into `phenodag_v3.go` (41,034 B) | DONE |
| `dagctl_v3_extend2.go` (11,648 B) | merged into `phenodag_v3.go` | DONE |
| `dagctl_v3_extend3.go` (18,811 B) | merged into `phenodag_v3.go` | DONE |
| `dagctl_remote_claim.go` (9,477 B) | merged into `phenodag.go` + `gh_repo_lease.go` | DONE |
| `dagctl_dedup2.go` (6,113 B) | merged into `phenodag_dedup2.go` (3,407 B) | DONE (smaller — content deduplicated) |
| `dagctl_extras.go` (11,300 B) | merged into `phenodag_extras.go` (42,947 B) | DONE (larger — extra phenodag content) |
| `dagctl_meta2.go` (6,720 B) | merged into `phenodag.go` | DONE |
| `dagctl_test2.go` (8,527 B) | merged into `phenodag_test.go` (8,782 B) | DONE |
| `dagctl_viz2.go` (4,848 B) | merged into `phenodag.go` | DONE |
| `dagctl_dag_template.html` (4,828 B) | copied as-is | DONE |
| `README.md` (6,551 B) | rewritten as `phenodag/README.md` (7,716 B) | DONE |
| `VERSION` (v3.3.1) | created at phenodag root | DONE (this PR) |

## Source repos

- `KooshaPari/dagctl` — **ARCHIVED** (read-only); 62 KB, default branch `main`
- `KooshaPari/phenodag` — **ACTIVE**; 266 KB, default branch `main`, v0.3.0 → v3.3.1 absorption complete

## Manual delete commands (post-archive)

The active `gh` token has scopes `'gist', 'read:org', 'repo', 'workflow'` — **no `delete_repo`**. Archive is the only available action via the API. To complete the `DELETE_AFTER_PATCHES` verdict, run these commands via the GitHub UI or with a token that has `delete_repo` scope:

```bash
# Via UI: Settings → Danger Zone → "Delete this repository"
# Repository: KooshaPari/dagctl (62 KB, archived 2026-06-17)

# Via gh CLI with delete_repo scope:
# gh auth refresh -h github.com -s delete_repo
# gh repo delete KooshaPari/dagctl --yes
```

90-day GitHub retention applies to the soft-delete tombstone.

## Related

- `KooshaPari/phenotype-registry` PR #151 — store dagctl v3.3.1 final tag patch
- `KooshaPari/phenotype-registry` PR #144 — kooshapari-absorption audit doc
- `findings/2026-06-17-L5-104-dmouse92-to-kooshapari.md` — broader dmouse92 → kooshapari migration matrix
- `findings/30-pillar-2026-06-16.md` — dagctl architecture pillar (L5-L8)