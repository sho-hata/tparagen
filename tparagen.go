package tparagen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/saracen/walker"
)

const (
	defaultTargetDir     = "./"
	defaultIgnoreDir     = "testdata"
	fixingForLoopVersion = 1.22
)

// Run is entry point.
func Run(ctx context.Context, outStream, errStream io.Writer, ignoreDirectories []string, minGoVersion float64) error {
	ignoreDirs := []string{defaultIgnoreDir}
	if len(ignoreDirs) != 0 {
		ignoreDirs = append(ignoreDirs, ignoreDirectories...)
	}

	t := &tparagen{
		in:         defaultTargetDir,
		dest:       "",
		outStream:  outStream,
		errStream:  errStream,
		ignoreDirs: ignoreDirs,
	}

	if minGoVersion < fixingForLoopVersion {
		t.needFixLoopVar = true
	}

	return t.run(ctx)
}

type tparagen struct {
	in, dest             string
	outStream, errStream io.Writer
	ignoreDirs           []string
	needFixLoopVar       bool
}

func (t *tparagen) run(ctx context.Context) error {
	// Rewritten contents keyed by file path. The walk itself is read-only:
	// files are written only after every file has been processed successfully,
	// so a failure half-way leaves every original untouched. Creating files
	// inside the directories being walked would also disturb the concurrent
	// directory listing. walker.Walk() may execute concurrently, so sync.Map
	// is used.
	var changes sync.Map

	if err := walker.Walk(t.in, func(path string, info fs.FileInfo) error {
		// Abort the scan early on interruption (SIGINT/SIGTERM); nothing has
		// been written at this point.
		if err := ctx.Err(); err != nil {
			return err
		}

		if info.IsDir() && t.skipDir(path) {
			return filepath.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		if filepath.Ext(path) != ".go" {
			return nil
		}

		if !strings.HasSuffix(filepath.Base(path), "_test.go") {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("cannot read %s. %w", path, err)
		}

		got, err := GenerateTParallel(path, src, t.needFixLoopVar)
		if err != nil {
			return fmt.Errorf("error occurred in Process(). %w", err)
		}

		if !bytes.Equal(src, got) {
			changes.Store(path, got)
		}

		return nil
	}); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("interrupted before applying changes: %w", err)
		}

		return fmt.Errorf("error occurred in walker.Walk(). %w", err)
	}

	// Do not begin the write phase if we were interrupted during the scan.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("interrupted before applying changes: %w", err)
	}

	// Write every file now that every rewrite succeeded. This phase runs to
	// completion without checking for cancellation so that the files are not
	// left in a partially rewritten state.
	return applyChanges(&changes)
}

// writeTempFile writes content to a new temporary file next to path, with the
// same permission bits as path, and returns its name. Creating it in the same
// directory keeps the final rename atomic and on the same file system: a
// temporary file under os.TempDir() cannot be renamed across devices (e.g.
// a tmpfs /tmp), and os.CreateTemp's fixed 0600 mode would replace the
// original permissions.
func writeTempFile(path string, content []byte) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cannot stat %s. %w", path, err)
	}

	tmpf, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tparagen-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for %s. %w", path, err)
	}

	tmpPath := tmpf.Name()

	fail := func(err error) (string, error) {
		tmpf.Close()
		os.Remove(tmpPath)

		return "", err
	}

	if err := tmpf.Chmod(info.Mode().Perm()); err != nil {
		return fail(fmt.Errorf("failed to set permissions of temp file for %s. %w", path, err))
	}

	if _, err := tmpf.Write(content); err != nil {
		return fail(fmt.Errorf("failed to write temp file for %s. %w", path, err))
	}

	if err := tmpf.Close(); err != nil {
		os.Remove(tmpPath)

		return "", fmt.Errorf("failed to close temp file for %s. %w", path, err)
	}

	return tmpPath, nil
}

// applyChanges writes every rewritten content (keyed by file path) to a
// temporary file next to its original and then renames the temporary files
// over the originals. All temporary files are written before the first rename,
// so a write failure (read-only directory, disk full, ...) leaves every
// original untouched. Every rename is attempted even if one fails, and all
// failures are reported so that the caller exits non-zero instead of silently
// pretending the files were updated.
func applyChanges(changes *sync.Map) error {
	type pendingFile struct {
		origPath, tmpPath string
	}

	var (
		pending  []pendingFile
		writeErr error
	)

	changes.Range(func(key, value any) bool {
		origPath, ok := key.(string)
		if !ok {
			return false
		}

		content, ok := value.([]byte)
		if !ok {
			return false
		}

		tmpPath, err := writeTempFile(origPath, content)
		if err != nil {
			writeErr = err

			return false
		}

		pending = append(pending, pendingFile{origPath: origPath, tmpPath: tmpPath})

		return true
	})

	if writeErr != nil {
		for _, p := range pending {
			os.Remove(p.tmpPath)
		}

		return writeErr
	}

	var errs []error

	for _, p := range pending {
		if err := os.Rename(p.tmpPath, p.origPath); err != nil {
			os.Remove(p.tmpPath)
			errs = append(errs, fmt.Errorf("failed to replace %s. %w", p.origPath, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("some files were not updated. %w", errors.Join(errs...))
	}

	return nil
}

func (t *tparagen) skipDir(p string) bool {
	for _, dir := range t.ignoreDirs {
		if filepath.Base(p) == dir {
			return true
		}
	}

	return false
}
