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
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/saracen/walker"
)

const (
	defaultTargetDir = "./"
	defaultIgnoreDir = "testdata"
)

// Run is entry point.
//
// minGoVersion is the Go language version the processed tests are written for
// (e.g. "1.21"). It decides whether loop variables have to be copied before a
// parallel subtest, which is needed before Go 1.22. When it is empty, the go
// directive of the go.mod nearest to each file is used.
//
// A summary of the changes is written to outStream; with verbose, also one
// line for every file that was changed or has tests left sequential.
func Run(ctx context.Context, outStream, errStream io.Writer, ignoreDirectories []string, minGoVersion string, verbose bool) error {
	ignoreDirs := []string{defaultIgnoreDir}
	if len(ignoreDirs) != 0 {
		ignoreDirs = append(ignoreDirs, ignoreDirectories...)
	}

	// Reject an invalid version before looking at any file.
	if minGoVersion != "" {
		if _, err := needFixLoopVar(minGoVersion); err != nil {
			return err
		}
	}

	t := &tparagen{
		in:           defaultTargetDir,
		dest:         "",
		outStream:    outStream,
		errStream:    errStream,
		ignoreDirs:   ignoreDirs,
		minGoVersion: minGoVersion,
		verbose:      verbose,
	}

	return t.run(ctx)
}

type tparagen struct {
	in, dest             string
	outStream, errStream io.Writer
	ignoreDirs           []string
	verbose              bool

	// minGoVersion overrides the go directive of every go.mod when set.
	minGoVersion string
	modules      moduleFinder
	warnNoGoMod  sync.Once
}

func (t *tparagen) run(ctx context.Context) error {
	// Rewritten contents keyed by file path. The walk itself is read-only:
	// files are written only after every file has been processed successfully,
	// so a failure half-way leaves every original untouched. Creating files
	// inside the directories being walked would also disturb the concurrent
	// directory listing. walker.Walk() may execute concurrently, so sync.Map
	// is used.
	var (
		changes sync.Map
		// fileStats of every file where something was done or left
		// sequential, keyed by file path.
		stats sync.Map
		// Number of test files read.
		scanned atomic.Int64
	)

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

		scanned.Add(1)

		needFix, err := t.needFixLoopVarIn(filepath.Dir(path))
		if err != nil {
			return err
		}

		got, s, err := generateTParallel(path, src, needFix)
		if err != nil {
			return fmt.Errorf("error occurred in Process(). %w", err)
		}

		if !bytes.Equal(src, got) {
			s.rewritten = true

			changes.Store(path, got)
		}

		if s != (fileStats{}) {
			stats.Store(path, s)
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
	if err := applyChanges(&changes); err != nil {
		return err
	}

	t.report(&stats, int(scanned.Load()))

	return nil
}

// report writes a summary of the run to outStream. One line per file would
// drown the summary in a large repository, so per-file lines are written only
// with verbose.
func (t *tparagen) report(stats *sync.Map, scanned int) {
	var (
		paths   []string
		total   fileStats
		updated int
	)

	stats.Range(func(key, value any) bool {
		path, _ := key.(string)
		s, _ := value.(fileStats)

		paths = append(paths, path)
		total.add(s)

		if s.rewritten {
			updated++
		}

		return true
	})

	slices.Sort(paths)

	if t.verbose {
		for _, path := range paths {
			v, _ := stats.Load(path)
			s, _ := v.(fileStats)
			fmt.Fprintf(t.outStream, "%s: %s\n", filepath.Clean(path), describeStats(s))
		}
	}

	if updated == 0 {
		fmt.Fprintf(t.outStream, "No changes in %s\n", plural(scanned, "test file", "test files"))
	} else {
		fmt.Fprintf(t.outStream, "Updated %d of %s: %s\n", updated, plural(scanned, "test file", "test files"),
			describeStats(fileStats{parallel: total.parallel, loopVarCopies: total.loopVarCopies, rewritten: true}))
	}

	if total.sequential > 0 {
		hint := ""
		if !t.verbose {
			hint = " Run with -v to see the files."
		}

		fmt.Fprintf(t.outStream, "%s left sequential: t.Parallel() is not safe there (t.Setenv, global state, ...).%s\n",
			plural(total.sequential, "test", "tests"), hint)
	}
}

// describeStats describes the non-zero counts of s, e.g.
// "3 t.Parallel() added, 1 loop variable copied".
func describeStats(s fileStats) string {
	var parts []string

	if s.parallel > 0 {
		parts = append(parts, fmt.Sprintf("%d t.Parallel() added", s.parallel))
	}

	if s.loopVarCopies > 0 {
		parts = append(parts, plural(s.loopVarCopies, "loop variable", "loop variables")+" copied")
	}

	if s.sequential > 0 {
		parts = append(parts, plural(s.sequential, "test", "tests")+" left sequential")
	}

	if len(parts) == 0 && s.rewritten {
		parts = append(parts, "reformatted by gofmt")
	}

	return strings.Join(parts, ", ")
}

// plural formats n with the singular or the plural form of a noun.
func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}

	return fmt.Sprintf("%d %s", n, pluralForm)
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
