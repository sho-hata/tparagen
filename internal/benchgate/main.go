// Command benchgate reads `benchstat -format csv` output and exits non-zero
// when a benchmark got significantly worse.
package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// alpha is benchstat's default significance level; benchstat prints "~" instead
// of a delta when p >= alpha, but the gate checks p itself rather than relying
// on that.
const alpha = 0.05

// maxRegressionPercent is how much worse, in percent, each unit may get before
// the gate fails. Units not listed here (e.g. B/s) are ignored.
var maxRegressionPercent = map[string]float64{
	"sec/op":     10,
	"B/op":       10,
	"allocs/op":  10,
	"peak-RSS-B": 10,
}

type regression struct {
	benchmark, unit string
	deltaPercent, p float64
}

func (r regression) String() string {
	return fmt.Sprintf("%s %s %+.2f%% (p=%.3f, limit +%.0f%%)", r.benchmark, r.unit, r.deltaPercent, r.p, maxRegressionPercent[r.unit])
}

// check scans benchstat CSV for significant regressions beyond
// maxRegressionPercent. It fails when it finds no benchmark row at all, so a
// change in benchstat's output format cannot turn the gate into a silent pass.
func check(r io.Reader) ([]regression, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1

	var (
		found       int
		regressions []regression
		unit        string
	)

	for {
		row, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, err
		}

		// A table header looks like ",sec/op,CI,sec/op,CI,vs base,P".
		if len(row) > 1 && row[0] == "" {
			unit = row[1]

			continue
		}

		// Data rows: name, old, CI, new, CI, delta, "p=0.012 n=10". The summary
		// row is not a benchmark.
		limit, ok := maxRegressionPercent[unit]
		if !ok || len(row) < 7 || row[0] == "geomean" {
			continue
		}

		found++

		if row[5] == "~" {
			continue
		}

		delta, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimPrefix(row[5], "+"), "%"), 64)
		if err != nil {
			return nil, fmt.Errorf("%s %s: cannot parse delta %q", row[0], unit, row[5])
		}

		var p float64
		if _, err := fmt.Sscanf(row[6], "p=%f", &p); err != nil {
			return nil, fmt.Errorf("%s %s: cannot parse p-value %q", row[0], unit, row[6])
		}

		if p < alpha && delta > limit {
			regressions = append(regressions, regression{row[0], unit, delta, p})
		}
	}

	if found == 0 {
		return nil, errors.New("no benchmark rows found in benchstat CSV")
	}

	return regressions, nil
}

func main() {
	in := io.Reader(os.Stdin)

	if len(os.Args) > 1 {
		f, err := os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "benchgate:", err)
			os.Exit(2)
		}
		defer f.Close()

		in = f
	}

	regressions, err := check(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchgate:", err)
		os.Exit(2)
	}

	for _, r := range regressions {
		fmt.Println("REGRESSION", r)
	}

	if len(regressions) > 0 {
		os.Exit(1)
	}

	fmt.Println("no significant regression")
}
