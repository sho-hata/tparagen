package tparagen

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// src is a test file that would be rewritten to insert t.Parallel().
const rewritableTestSrc = `package t

import "testing"

func TestFoo(t *testing.T) {
	t.Run("1", func(t *testing.T) {
	})
}
`

// testGoMod is the go.mod of the temporary modules the tests run on.
const testGoMod = "module example.com/m\n\ngo 1.26\n"

// setupTestModule writes a go.mod and a rewritable _test.go file into a
// temporary directory and returns the file path together with its original
// contents.
func setupTestModule(t *testing.T) (string, []byte) {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(testGoMod), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	path := filepath.Join(dir, "foo_test.go")
	if err := os.WriteFile(path, []byte(rewritableTestSrc), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	return path, []byte(rewritableTestSrc)
}

// writeFiles writes files, keyed by their path relative to root.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// newRunner builds a tparagen that processes the packages matched by patterns
// (./... when none is given) in dir, for exercising run() directly.
func newRunner(dir string, patterns ...string) *tparagen {
	if len(patterns) == 0 {
		patterns = []string{defaultPattern}
	}

	return &tparagen{
		dir:        dir,
		patterns:   patterns,
		outStream:  io.Discard,
		errStream:  io.Discard,
		ignoreDirs: []string{defaultIgnoreDir},
	}
}

func TestRunAppliesChanges(t *testing.T) {
	t.Parallel()

	path, orig := setupTestModule(t)

	if err := newRunner(filepath.Dir(path)).run(context.Background()); err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(got) == string(orig) {
		t.Fatal("expected file to be rewritten, but it was unchanged")
	}
}

func TestRunLeavesFilesUntouchedWhenCanceled(t *testing.T) {
	t.Parallel()

	path, orig := setupTestModule(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before run starts

	err := newRunner(filepath.Dir(path)).run(ctx)
	if err == nil {
		t.Fatal("expected an error when context is canceled, got nil")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(got) != string(orig) {
		t.Fatalf("expected file to be untouched on cancellation.\norig:\n%s\ngot:\n%s", orig, got)
	}
}

func TestRunPreservesFileMode(t *testing.T) {
	t.Parallel()

	path, _ := setupTestModule(t)

	const want = os.FileMode(0o640)
	if err := os.Chmod(path, want); err != nil {
		t.Fatal(err)
	}

	if err := newRunner(filepath.Dir(path)).run(context.Background()); err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if got := info.Mode().Perm(); got != want {
		t.Errorf("file mode after run = %o, want %o", got, want)
	}
}

func TestRunLeavesNoTemporaryFiles(t *testing.T) {
	t.Parallel()

	path, _ := setupTestModule(t)

	if err := newRunner(filepath.Dir(path)).run(context.Background()); err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}

		t.Errorf("unexpected files left in the target directory: %v", names)
	}
}

func TestWriteTempFileIsCreatedNextToTarget(t *testing.T) {
	t.Parallel()

	path, _ := setupTestModule(t)

	tmpPath, err := writeTempFile(path, []byte("package t\n"))
	if err != nil {
		t.Fatal(err)
	}

	if filepath.Dir(tmpPath) != filepath.Dir(path) {
		t.Errorf("temp file %s is not in the target directory %s", tmpPath, filepath.Dir(path))
	}

	if filepath.Ext(tmpPath) == ".go" {
		t.Errorf("temp file %s must not look like a Go file", tmpPath)
	}

	got, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "package t\n" {
		t.Errorf("temp file content = %q", got)
	}
}

func TestRunLeavesAllFilesUntouchedWhenOneCannotBeWritten(t *testing.T) {
	t.Parallel()

	if os.Getuid() == 0 {
		t.Skip("root is not subject to directory permissions")
	}

	root := t.TempDir()
	writeFiles(t, root, map[string]string{"go.mod": testGoMod})

	writable := filepath.Join(root, "ok")
	readOnly := filepath.Join(root, "ro")

	for _, dir := range []string{writable, readOnly} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(dir, "foo_test.go"), []byte(rewritableTestSrc), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// A read-only directory makes writing the temporary file for
	// ro/foo_test.go fail; ok/foo_test.go must then stay untouched as well.
	if err := os.Chmod(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o755) })

	if err := newRunner(root).run(context.Background()); err == nil {
		t.Fatal("expected an error when a file cannot be written, got nil")
	}

	for _, dir := range []string{writable, readOnly} {
		got, err := os.ReadFile(filepath.Join(dir, "foo_test.go"))
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != rewritableTestSrc {
			t.Errorf("%s/foo_test.go was modified although the run failed", dir)
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}

		if len(entries) != 1 {
			t.Errorf("temporary files left behind in %s: %d entries", dir, len(entries))
		}
	}
}

// Every module of a workspace is processed with the Go version of its own
// go.mod: loop variables are copied only in the module that is older than
// Go 1.22.
func TestRunUsesTheGoVersionOfEachModule(t *testing.T) {
	t.Parallel()

	const src = `package t

import "testing"

func TestFoo(t *testing.T) {
	for _, tc := range []string{"a"} {
		t.Run(tc, func(t *testing.T) {
			_ = tc
		})
	}
}
`

	root := t.TempDir()

	writeFiles(t, root, map[string]string{
		"go.work":           "go 1.22\n\nuse (\n\t.\n\t./old\n)\n",
		"go.mod":            "module m\n\ngo 1.22\n",
		"new/foo_test.go":   src,
		"old/go.mod":        "module m/old\n\ngo 1.21\n",
		"old/a/foo_test.go": src,
	})

	// "./..." stops at the nested module, like it does for the go command.
	if err := newRunner(root, "./...", "./old/...").run(context.Background()); err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	for name, wantCopy := range map[string]bool{"new/foo_test.go": false, "old/a/foo_test.go": true} {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}

		if hasCopy := strings.Contains(string(got), "tc := tc"); hasCopy != wantCopy {
			t.Errorf("%s: loop variable copied = %v, want %v\n%s", name, hasCopy, wantCopy, got)
		}
	}
}

// Run processes the packages that the patterns select, as the go command
// resolves them, and leaves every other file alone.
func TestRunProcessesOnlyMatchedPackages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		patterns    []string
		ignoreDirs  []string
		buildTags   []string
		wantChanged []string
	}{
		{
			name:        "./... covers the main module only",
			patterns:    []string{"./..."},
			wantChanged: []string{"a/foo_test.go", "a/sub/foo_test.go", "b/foo_test.go", "tagged/foo_test.go"},
		},
		{
			name:        "build tags match a directory whose files all need them",
			patterns:    []string{"./..."},
			buildTags:   []string{"e2e"},
			wantChanged: []string{"a/foo_test.go", "a/sub/foo_test.go", "b/foo_test.go", "tagged/foo_test.go", "e2e/foo_test.go"},
		},
		{
			name:        "a directory whose files all need a tag, by its path",
			patterns:    []string{"./e2e"},
			wantChanged: []string{"e2e/foo_test.go"},
		},
		{
			name:        "a subtree",
			patterns:    []string{"./a/..."},
			wantChanged: []string{"a/foo_test.go", "a/sub/foo_test.go"},
		},
		{
			name:        "a single package by its import path",
			patterns:    []string{"example.com/m/a"},
			wantChanged: []string{"a/foo_test.go"},
		},
		{
			name:        "several patterns",
			patterns:    []string{"./b", "./tagged"},
			wantChanged: []string{"b/foo_test.go", "tagged/foo_test.go"},
		},
		{
			name:        "ignored directories",
			patterns:    []string{"./..."},
			ignoreDirs:  []string{"sub", "b"},
			wantChanged: []string{"a/foo_test.go", "tagged/foo_test.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			files := map[string]string{
				"go.mod":               testGoMod,
				"a/foo_test.go":        rewritableTestSrc,
				"a/sub/foo_test.go":    rewritableTestSrc,
				"b/foo_test.go":        rewritableTestSrc,
				"tagged/foo_test.go":   "//go:build integration\n\n" + rewritableTestSrc,
				"tagged/tagged.go":     "package t\n",
				"e2e/foo_test.go":      "//go:build e2e\n\n" + rewritableTestSrc,
				"testdata/foo_test.go": rewritableTestSrc,
				"vendor/v/foo_test.go": rewritableTestSrc,
				".hidden/foo_test.go":  rewritableTestSrc,
				"_skip/foo_test.go":    rewritableTestSrc,
				"nested/go.mod":        "module example.com/nested\n\ngo 1.26\n",
				"nested/n/foo_test.go": rewritableTestSrc,
			}

			root := t.TempDir()
			writeFiles(t, root, files)

			tp := newRunner(root, tt.patterns...)
			tp.ignoreDirs = append(tp.ignoreDirs, tt.ignoreDirs...)
			tp.buildTags = tt.buildTags

			if err := tp.run(context.Background()); err != nil {
				t.Fatalf("run() returned error: %v", err)
			}

			for name, orig := range files {
				if !strings.HasSuffix(name, "_test.go") {
					continue
				}

				got, err := os.ReadFile(filepath.Join(root, name))
				if err != nil {
					t.Fatal(err)
				}

				changed := string(got) != orig
				if want := slices.Contains(tt.wantChanged, name); changed != want {
					t.Errorf("%s: changed = %v, want %v", name, changed, want)
				}
			}
		})
	}
}

// A pattern that matches no package is an error, and nothing is rewritten.
func TestRunFailsOnPatternsMatchingNothing(t *testing.T) {
	t.Parallel()

	for _, pattern := range []string{"./missing", "./missing/..."} {
		t.Run(pattern, func(t *testing.T) {
			t.Parallel()

			path, orig := setupTestModule(t)

			err := newRunner(filepath.Dir(path), "./...", pattern).run(context.Background())
			if err == nil {
				t.Fatal("expected an error for a pattern matching nothing, got nil")
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != string(orig) {
				t.Errorf("file was rewritten although the run failed:\n%s", got)
			}
		})
	}
}
