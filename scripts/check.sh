#!/usr/bin/env bash
# Local quality gate. Mirrors CI (ci.yml) so broken changes are caught
# BEFORE they are committed or pushed.
#
# Usage:
#   scripts/check.sh
#
# Exits non-zero on the first failing check. Install as a pre-commit hook:
#   ln -sf ../../scripts/check.sh .git/hooks/pre-commit
#
# Uses the ambient Go toolchain (no hardcoded GOROOT).

set -uo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)"
[ -n "$ROOT" ] || { echo "FAILED: not inside a git repository" >&2; exit 1; }
cd "$ROOT" || exit 1

fail() { echo "FAILED: $1" >&2; exit 1; }

echo "==> gofmt"
out=$(gofmt -l .)
[ -z "$out" ] || fail "gofmt found unformatted files:
$out"

echo "==> go vet"
go vet ./... || fail "go vet failed"

echo "==> go build"
go build ./... || fail "go build failed"

echo "==> go test -race"
go test -race ./... || fail "go test -race failed"

echo "OK: all checks passed"
