#!/bin/sh
# scripts/verify-no-port-dupes.sh
# POSIX sh; no external deps (grep, awk, sed).
#
# Detects duplicated `*Port`-suffixed function declarations across the
# phenodag superset-merge source files. The phenodag+dagctl superset
# introduces `_Port` aliases for the v3 and dedup2 wave; this guard
# fails CI if any `*_Port` function is reintroduced without a matching
# deprecation note in the FR superset-merge tracker (phenodag#5).
#
# Usage:
#   sh scripts/verify-no-port-dupes.sh [path ...]
#   Default path: . (current directory)
#
# Exit codes:
#   0  PASS  - no duplicate *_Port functions found, or all duplicates
#              are explicitly listed in scripts/.port-dupes-allowlist
#   1  FAIL  - duplicate *_Port function(s) detected
#   2  USAGE - bad invocation

set -u

root=${1:-.}
allowlist="${root}/scripts/.port-dupes-allowlist"

if [ ! -d "${root}" ]; then
    echo "verify-no-port-dupes: ${root} is not a directory" >&2
    exit 2
fi

# Collect function declarations that end in "Port(" (capital P, then open paren)
# across the 5 superset-merge source files. Uses grep -E extended regex.
found=$(grep -nE '^func[[:space:]]+[A-Za-z_][A-Za-z0-9_]*Port\(' \
    "${root}/phenodag.go" \
    "${root}/phenodag_v3.go" \
    "${root}/phenodag_dedup2.go" \
    "${root}/phenodag_extras.go" \
    "${root}/queries.go" 2>/dev/null | \
    awk -F: '{print $1":"$3}' | sort || true)

if [ -z "${found}" ]; then
    echo "verify-no-port-dupes: PASS - 0 *Port duplicates found"
    exit 0
fi

# If an allowlist exists, filter found list against it.
if [ -f "${allowlist}" ]; then
    filtered=""
    while IFS= read -r line; do
        [ -z "${line}" ] && continue
        case "${line}" in \#*) continue ;; esac
        match=$(printf '%s\n' "${found}" | grep -F "${line}" || true)
        if [ -n "${match}" ]; then
            filtered="${filtered}${match}\n"
        fi
    done < "${allowlist}"
    remaining=$(printf '%s\n' "${found}" | grep -v -F -f "${allowlist}" || true)
else
    remaining="${found}"
fi

if [ -z "${remaining}" ]; then
    echo "verify-no-port-dupes: PASS - all ${#found} *Port duplicates are in allowlist"
    exit 0
fi

echo "verify-no-port-dupes: FAIL - ${#remaining} duplicate *Port function(s) detected:" >&2
printf '%s\n' "${remaining}" >&2
echo "" >&2
echo "Each *Port function is a v3/dedup2 wave alias that should be merged into" >&2
echo "the canonical non-Port version per phenodag#5 (Phase-4b superset-merge)." >&2
echo "Either:" >&2
echo "  (a) Remove the *Port function and update all callers to use the canonical name" >&2
echo "  (b) Add the (file:function) tuple to scripts/.port-dupes-allowlist with rationale" >&2
echo "" >&2
echo "See phenodag#5 for the superset-merge tracker." >&2
exit 1
