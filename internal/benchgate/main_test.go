package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The fixtures are real `benchstat -format csv` output over seeded inputs; see
// testdata/gen.sh. Each seeded case differs from base.txt in one way.
func TestCheckControls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase string
		fixture  string
		want     []string
	}{
		{"30% sec/op regression with p<0.05 fails", "sec_regress", []string{"Run-4 sec/op +30.00%", "Parse-4 sec/op +30.00%"}},
		{"15% allocs/op regression fails", "allocs_regress", []string{"Run-4 allocs/op +15.00%", "Parse-4 allocs/op +14.00%"}},
		{"identical inputs pass", "identical", nil},
		{"30% improvement passes", "improvement", nil},
	}

	for _, tt := range tests {
		t.Run(tt.testCase, func(t *testing.T) {
			t.Parallel()

			f, err := os.Open(filepath.Join("testdata", tt.fixture+".csv"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			got, err := check(f)
			if err != nil {
				t.Fatal(err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %d regressions", got, len(tt.want))
			}

			for i, w := range tt.want {
				if !strings.HasPrefix(got[i].String(), w) {
					t.Errorf("regression %d = %q, want prefix %q", i, got[i], w)
				}
			}
		})
	}
}

func TestCheckThresholds(t *testing.T) {
	t.Parallel()

	row := func(unit, delta, p string) string {
		return ",old,,new,,,\n," + unit + ",CI," + unit + ",CI,vs base,P\nRun-4,1,0%,2,0%," + delta + "," + p + " n=10\n"
	}

	tests := []struct {
		testCase  string
		unit      string
		delta     string
		p         string
		wantFails bool
	}{
		{"sec/op at the limit passes", "sec/op", "+20.00%", "p=0.001", false},
		{"sec/op above the limit fails", "sec/op", "+20.01%", "p=0.001", true},
		{"B/op above the limit fails", "B/op", "+10.50%", "p=0.001", true},
		{"B/op below the limit passes", "B/op", "+9.00%", "p=0.001", false},
		{"allocs/op above the limit fails", "allocs/op", "+10.50%", "p=0.001", true},
		{"insignificant p passes", "sec/op", "+50.00%", "p=0.050", false},
		{"unlisted unit is ignored", "B/s", "+90.00%", "p=0.001", false},
	}

	for _, tt := range tests {
		t.Run(tt.testCase, func(t *testing.T) {
			t.Parallel()

			// A second, unflagged unit keeps "found" true for ignored-unit cases.
			src := row(tt.unit, tt.delta, tt.p) + "\n" + row("allocs/op", "~", "p=1.000")

			got, err := check(strings.NewReader(src))
			if err != nil {
				t.Fatal(err)
			}

			if (len(got) > 0) != tt.wantFails {
				t.Errorf("got %v, wantFails=%v", got, tt.wantFails)
			}
		})
	}
}

func TestCheckRejectsOutputWithoutBenchmarks(t *testing.T) {
	t.Parallel()

	for _, src := range []string{"", "goos: linux\n", ",old,,new,,,\n,sec/op,CI,sec/op,CI,vs base,P\ngeomean,1,,1,,+0.00%,\n"} {
		if _, err := check(strings.NewReader(src)); err == nil {
			t.Errorf("check(%q) = nil error, want one", src)
		}
	}
}

// The checked-in CSV must be what the pinned benchstat prints for the seeded
// inputs, otherwise the controls above would test a format nobody produces.
// Set BENCHSTAT to a benchstat binary to run it.
func TestFixturesMatchBenchstat(t *testing.T) {
	bin := os.Getenv("BENCHSTAT")
	if bin == "" {
		t.Skip("BENCHSTAT not set")
	}

	for _, name := range []string{"identical", "sec_regress", "allocs_regress", "improvement"} {
		cmd := exec.Command(bin, "-format", "csv", "base.txt", name+".txt")
		cmd.Dir = "testdata"

		var out bytes.Buffer
		cmd.Stdout = &out

		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}

		want, err := os.ReadFile(filepath.Join("testdata", name+".csv"))
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(out.Bytes(), want) {
			t.Errorf("%s.csv is stale; rerun testdata/gen.sh", name)
		}
	}
}
