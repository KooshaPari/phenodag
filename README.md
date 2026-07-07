# phenodag

> **DEPRECATED -- thin redirector only. Archive target: 2026-07-12 (1 release cycle from 2026-07-05).**

phenodag has been **absorbed** into the Phenotype polyrepo portfolio. The
DAG / queue / atomic-claim / lease surface lives in
**[Tracera](https://github.com/KooshaPari/Tracera)** and the PM / cockpit /
portfolio surface lives in
**[AgilePlus](https://github.com/KooshaPari/AgilePlus)**.

## Where to go now

| Capability              | New home                                      |
| ----------------------- | --------------------------------------------- |
| DAG execution           | https://github.com/KooshaPari/Tracera         |
| Queue (claim/heartbeat) | https://github.com/KooshaPari/Tracera         |
| Atomic claim / lease    | https://github.com/KooshaPari/Tracera         |
| Presets / configs       | https://github.com/KooshaPari/Tracera         |
| Project / portfolio PM  | https://github.com/KooshaPari/AgilePlus       |
| Cockpit / dashboard     | https://github.com/KooshaPari/AgilePlus       |

Both Tracera (spec 008) and AgilePlus (spec 008) contain the phenodag FR map.

## Status

- 2026-07-05: thin redirector placed (see CHANGELOG).
- 2026-07-05: spec-level absorption complete -- Tracera PRs #723, #725, #727 and
  AgilePlus #895, #896 all merged.
- ~2026-07-12: this repo will be archived on GitHub.
- After archive: read-only. The `phenodag` package on crates.io / Go modules
  will be marked deprecated; switch to Tracera.

## For new users

Do not start new work here. Use Tracera and AgilePlus.

## Context

Part of the polyrepo portfolio strategy session 2026-07-05.
See `docs/sessions/2026-07-05-polyrepo-portfolio-strategy/` in the Phenotype
repos monorepo for the full decision tree.
