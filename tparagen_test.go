package tparagen

import (
	"context"
	"io"
	"os"
	"path/filepath"
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

// setupTestModule writes a rewritable _test.go file into a temporary directory
// and returns the file path together with its original contents.
func setupTestModule(t *testing.T) (string, []byte) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "foo_test.go")
	if err := os.WriteFile(path, []byte(rewritableTestSrc), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	return path, []byte(rewritableTestSrc)
}

// newRunner builds a tparagen rooted at dir for exercising run() directly.
func newRunner(dir string) *tparagen {
	return &tparagen{
		in:         dir,
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

	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
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

// Every module of a multi-module repository is processed with the Go version
// of its own go.mod: loop variables are copied only in the module that is
// older than Go 1.22.
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

	files := map[string]string{
		"go.mod":            "module m\n\ngo 1.22\n",
		"new/foo_test.go":   src,
		"old/go.mod":        "module m/old\n\ngo 1.21\n",
		"old/a/foo_test.go": src,
	}

	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := newRunner(root).run(context.Background()); err != nil {
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
