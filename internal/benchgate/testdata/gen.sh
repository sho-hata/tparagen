#!/usr/bin/env bash
# Regenerates the seeded benchmark inputs and the benchstat CSV the gate tests
# read. Needs the benchstat version pinned in .github/workflows/bench.yml on
# PATH, or its path in $BENCHSTAT. Run from any directory.
set -euo pipefail

cd "$(dirname "$0")"

# gen NS B_PER_OP ALLOCS RSS: ten samples per benchmark with a fixed +-0.5% jitter,
# so the output is identical on every run.
gen() {
  awk -v ns="$1" -v bop="$2" -v al="$3" -v rss="$4" 'BEGIN {
    split("1.000 1.004 0.997 1.002 0.995 1.003 0.998 1.001 1.005 0.996", j, " ")
    print "goos: linux\ngoarch: amd64\npkg: example.com/x\ncpu: seeded"
    for (i = 1; i <= 10; i++) {
      printf "BenchmarkRun-4\t100\t%d ns/op\t%d peak-RSS-B\t%d B/op\t%d allocs/op\n", ns * j[i], rss * j[i], bop, al
      printf "BenchmarkParse-4\t100\t%d ns/op\t%d peak-RSS-B\t%d B/op\t%d allocs/op\n", ns * j[11 - i] / 10, rss * j[11 - i] / 10, bop / 10, al / 10
    }
  }'
}

gen 1000000 50000 500 20000000 >base.txt
gen 1000000 50000 500 20000000 >identical.txt
gen 1300000 50000 500 20000000 >sec_regress.txt
gen 1000000 50000 575 20000000 >allocs_regress.txt
gen 1000000 50000 500 23000000 >rss_regress.txt
gen 700000 50000 500 14000000 >improvement.txt

for c in identical sec_regress allocs_regress rss_regress improvement; do
  "${BENCHSTAT:-benchstat}" -format csv base.txt "$c.txt" >"$c.csv" 2>/dev/null
done
