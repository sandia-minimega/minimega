#!/bin/bash

# Print per-package and total statement coverage from a Go cover profile as a
# Markdown table.

set -e

PROFILE=${1:?usage: $0 <coverprofile>}
MODULE=$(go list -m) || exit $?

echo "| Package | Coverage |"
echo "| --- | ---: |"

# Profile lines are "file:start,end statements count", after a mode header.
awk -v prefix="$MODULE/" '
NR == 1 { next }
{
    pkg = $1
    sub(/\/[^\/]*:.*$/, "", pkg)
    if (index(pkg, prefix) == 1) pkg = substr(pkg, length(prefix) + 1)
    total[pkg] += $2
    if ($3 > 0) covered[pkg] += $2
}
END {
    for (p in total) printf "%s %d %d\n", p, covered[p], total[p]
}' "$PROFILE" | sort | awk '
{
    printf "| `%s` | %.1f%% |\n", $1, 100 * $2 / $3
    hit += $2
    all += $3
}
END {
    printf "| **Total** | **%.1f%%** |\n", all ? 100 * hit / all : 0
}'
