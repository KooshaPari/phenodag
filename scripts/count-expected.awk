#!/usr/bin/awk -f
# count-expected.awk - compute expected task count for a phenodag preset YAML
# Usage: awk -f count-expected.awk presets/foo.yaml
# Returns: core.stages * core.width + sum(side_dags[].size)

BEGIN {
    c = 0; w = 0; s = 0
    in_core = 0; in_side = 0
}
/^[[:space:]]*$/      { next }
/^core:[[:space:]]*$/ {
    in_core = 1; in_side = 0
    next
}
/^side_dags:[[:space:]]*$/ {
    in_core = 0; in_side = 1
    next
}
in_core && /^[[:space:]]+stages:[[:space:]]+[0-9]+/ {
    c = $2 + 0
}
in_core && /^[[:space:]]+width:[[:space:]]+[0-9]+/ {
    w = $2 + 0
}
in_side && /^[[:space:]]+size:[[:space:]]+[0-9]+/ {
    s += $2 + 0
}
END {
    # core block may or may not exist; compute the core contribution from c*w
    # only if c and w are both set (avoid counting core when there's no core block).
    print c * w + s
}