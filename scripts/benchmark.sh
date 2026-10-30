#!/usr/bin/env bash
# End-to-end benchmark of the tparagen CLI over a generated corpus.
#
# Usage: scripts/benchmark.sh
#
# Environment (defaults in parentheses):
#   PACKAGES (200)  package directories in the corpus
#   FILES    (15)   test files per package
#   TESTS    (20)   test functions per file
#   DEPTH    (3)    nesting depth of the nested subtest shape
#   RUNS     (10)   timed runs
#   OUT      (benchmark-results.md)  markdown table written by hyperfine
#   HYPERFINE (hyperfine)   hyperfine executable
#
# The corpus is spread over many directories on purpose: tparagen walks
# directories concurrently, so one flat directory would only use one core.
set -euo pipefail

HYPERFINE=${HYPERFINE:-hyperfine}
if ! command -v "$HYPERFINE" >/dev/null 2>&1; then
  echo "hyperfine is required: https://github.com/sharkdp/hyperfine#installation" >&2
  echo "  macOS: brew install hyperfine    Debian/Ubuntu: apt install hyperfine    Rust: cargo install hyperfine" >&2
  exit 1
fi

PACKAGES=${PACKAGES:-200}
FILES=${FILES:-15}
TESTS=${TESTS:-20}
DEPTH=${DEPTH:-3}
RUNS=${RUNS:-10}
OUT=${OUT:-benchmark-results.md}

repo=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# Absolute, so it still points at the file after hyperfine changes directory.
case $OUT in
  /*) ;;
  *) OUT=$PWD/$OUT ;;
esac

cd "$repo"
go build -trimpath -o "$work/tparagen" ./cmd/tparagen
go build -trimpath -o "$work/benchcorpus" ./internal/benchcorpus/cmd/benchcorpus

# go 1.21 in go.mod makes tparagen copy loop variables, the more expensive path.
"$work/benchcorpus" -dir "$work/pristine" -packages "$PACKAGES" -files "$FILES" -tests "$TESTS" -depth "$DEPTH" -go-version 1.21

cpu=$(sysctl -n machdep.cpu.brand_string 2>/dev/null || awk -F': ' '/model name/ {print $2; exit}' /proc/cpuinfo 2>/dev/null || echo unknown)
cores=$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo unknown)
mb=$(du -sk "$work/pristine" | awk '{printf "%.1f", $1 / 1024}')

echo "go:     $(go version)"
echo "cpu:    $cpu ($cores logical cores)"
echo "corpus: $(find "$work/pristine" -name '*_test.go' | wc -l | tr -d ' ') files," \
  "$(cat "$work"/pristine/*/*_test.go | grep -c '^func Test') tests," \
  "$mb MB in $PACKAGES packages"

# Rewriting is idempotent: a second run over rewritten files does nothing, so
# every run starts from a fresh copy of the pristine corpus.
printf -v restore 'rm -rf %q && cp -R %q %q' "$work/run" "$work/pristine" "$work/run"
printf -v command 'cd %q && %q' "$work/run" "$work/tparagen"

"$HYPERFINE" --warmup 1 --runs "$RUNS" --prepare "$restore" --command-name tparagen --export-markdown "$OUT" "$command"

echo "wrote $OUT"
