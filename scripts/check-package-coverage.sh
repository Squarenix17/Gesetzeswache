#!/usr/bin/env bash
# Enforce per-package statement coverage floors (P4 D7).
set -euo pipefail

min_pct="${MIN_COVERAGE:-80}"
packages=(
  "./internal/service"
  "./internal/apihttp"
  "./internal/sync"
  "./internal/store"
)

fail=0
for pkg in "${packages[@]}"; do
  line=$(go test -cover "$pkg" 2>&1 | tail -1)
  pct=$(echo "$line" | sed -n 's/.*coverage: \([0-9.]*\)% of statements.*/\1/p')
  if [ -z "$pct" ]; then
    echo "failed to parse coverage for ${pkg}: ${line}" >&2
    fail=1
    continue
  fi
  echo "package ${pkg}: ${pct}% (floor ${min_pct}%)"
  awk -v p="$pct" -v f="$min_pct" 'BEGIN { if (p+0 < f+0) exit 1 }' || {
    echo "package ${pkg} coverage ${pct}% below floor ${min_pct}%" >&2
    fail=1
  }
done

exit "$fail"
