//go:build unix

package tparagen

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/sho-hata/tparagen/internal/benchcorpus"
)

// The benchmark names are what CI compares between the base and the head of a
// pull request. Renaming one drops it from the comparison.

func BenchmarkGenerateTParallel(b *testing.B) {
	benchmarks := []struct {
		name           string
		spec           benchcorpus.Spec
		needFixLoopVar bool
	}{
		{"typical", benchcorpus.Spec{TestsPerFile: 20, NestDepth: 3}, false},
		{"loopvar", benchcorpus.Spec{TestsPerFile: 20, NestDepth: 3}, true},
		{"deep", benchcorpus.Spec{TestsPerFile: 14, NestDepth: 30}, false},
	}

	for _, bm := range benchmarks {
		src := benchcorpus.File(bm.spec, 0, 0)

		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(src)))

			for b.Loop() {
				if _, err := GenerateTParallel("x_test.go", src, bm.needFixLoopVar); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// peakRSSChildEnv makes the test binary run tparagen once in its working
// directory and exit, so its peak RSS is that of a single real run.
const peakRSSChildEnv = "TPARAGEN_BENCH_CHILD"

func TestMain(m *testing.M) {
	if os.Getenv(peakRSSChildEnv) != "" {
		if err := Run(context.Background(), io.Discard, io.Discard, nil, "1.21"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		os.Exit(0)
	}

	os.Exit(m.Run())
}

// BenchmarkRun measures a whole run over a tree. Files are spread over many
// package directories because the walk is parallel per directory, so a flat
// corpus would only use one core. The two sizes expose cost that grows with
// the tree, such as Run holding every rewritten file until the end.
func BenchmarkRun(b *testing.B) {
	sizes := []struct {
		name     string
		packages int
	}{
		{"small", 20},
		{"large", 100},
	}

	for _, size := range sizes {
		spec := benchcorpus.Spec{
			Packages:        size.packages,
			FilesPerPackage: 10,
			TestsPerFile:    20,
			NestDepth:       3,
			GoVersion:       "1.21",
		}

		b.Run(size.name, func(b *testing.B) { benchmarkRun(b, spec) })
	}
}

// peakRSSRuns is how many child processes are measured. The median is
// reported: GC timing makes a single process's peak noisy, and the maximum
// would pass that noise through.
const peakRSSRuns = 5

// peakRSSCache holds one measurement per corpus shape. The testing package
// calls a benchmark function several times while it sizes b.N, and the
// measurement does not depend on b.N.
var peakRSSCache = map[benchcorpus.Spec]float64{}

func benchmarkRun(b *testing.B, spec benchcorpus.Spec) {
	pristine := b.TempDir()
	if err := benchcorpus.Write(pristine, spec); err != nil {
		b.Fatal(err)
	}

	want := expectedParallelCalls(b, pristine)

	work := b.TempDir()
	b.Chdir(work)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Rewriting is idempotent, so every iteration needs the pristine tree
		// back or it would measure a run that has nothing to do.
		b.StopTimer()
		resetDir(b, work)

		if err := os.CopyFS(work, os.DirFS(pristine)); err != nil {
			b.Fatal(err)
		}

		b.StartTimer()

		if err := Run(context.Background(), io.Discard, io.Discard, nil, "1.21"); err != nil {
			b.Fatal(err)
		}

		if i == 0 {
			b.StopTimer()

			if got := countParallelCalls(b, work, nil); got != want {
				b.Fatalf("rewrote %d .Parallel() calls, want %d: the benchmark did not do the work", got, want)
			}

			b.StartTimer()
		}
	}

	b.StopTimer()

	rss, ok := peakRSSCache[spec]
	if !ok {
		rss = peakRSS(b, pristine)
		peakRSSCache[spec] = rss
	}

	b.ReportMetric(rss, "peak-RSS-B")
}

// peakRSS runs tparagen once per child process over a fresh copy of the corpus
// and returns the median peak resident set size in bytes. B/op counts
// cumulative allocation, so only this sees a change that keeps more alive.
func peakRSS(b *testing.B, pristine string) float64 {
	b.Helper()

	peaks := make([]float64, 0, peakRSSRuns)

	for range peakRSSRuns {
		dir := b.TempDir()
		if err := os.CopyFS(dir, os.DirFS(pristine)); err != nil {
			b.Fatal(err)
		}

		cmd := exec.Command(os.Args[0])
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), peakRSSChildEnv+"=1")

		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("child run failed: %v\n%s", err, out)
		}

		ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
		if !ok {
			b.Fatal("no rusage for the child process")
		}

		rss := float64(ru.Maxrss)
		if runtime.GOOS != "darwin" {
			// Maxrss is in kilobytes everywhere but darwin, where it is bytes.
			rss *= 1024
		}

		peaks = append(peaks, rss)
	}

	slices.Sort(peaks)

	return peaks[len(peaks)/2]
}

func resetDir(b *testing.B, dir string) {
	b.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		b.Fatal(err)
	}

	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			b.Fatal(err)
		}
	}
}

// expectedParallelCalls counts the .Parallel() calls the corpus would contain
// after a run, computed in memory without touching the files.
func expectedParallelCalls(b *testing.B, dir string) int {
	b.Helper()

	return countParallelCalls(b, dir, func(path string, src []byte) []byte {
		got, err := GenerateTParallel(path, src, true)
		if err != nil {
			b.Fatal(err)
		}

		return got
	})
}

// countParallelCalls counts ".Parallel()" in every *_test.go under dir, after
// passing each file through transform when it is set.
func countParallelCalls(b *testing.B, dir string, transform func(path string, src []byte) []byte) int {
	b.Helper()

	total := 0

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if transform != nil {
			src = transform(path, src)
		}

		total += bytes.Count(src, []byte(".Parallel()"))

		return nil
	})
	if err != nil {
		b.Fatal(err)
	}

	return total
}
