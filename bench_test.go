package tparagen

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// BenchmarkRun measures a whole run over a tree. Files are spread over many
// package directories because the walk is parallel per directory, so a flat
// corpus would only use one core.
func BenchmarkRun(b *testing.B) {
	spec := benchcorpus.Spec{
		Packages:        20,
		FilesPerPackage: 10,
		TestsPerFile:    20,
		NestDepth:       3,
		GoVersion:       "1.21",
	}

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
