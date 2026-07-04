package tparagen

import (
	"context"
	"io"
	"os"
	"path/filepath"
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
