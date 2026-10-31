package tparagen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

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
func Run(ctx context.Context, outStream, errStream io.Writer, ignoreDirectories []string, minGoVersion string) error {
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
	}

	return t.run(ctx)
}

type tparagen struct {
	in, dest             string
	outStream, errStream io.Writer
	ignoreDirs           []string

	// minGoVersion overrides the go directive of every go.mod when set.
	minGoVersion string
	modules      moduleFinder
	warnNoGoMod  sync.Once
}

func (t *tparagen) run(ctx context.Context) error {
	// The run is read-only until every file has been processed successfully:
	// files are written only at the end, so a failure half-way leaves every
	// original untouched. Creating files inside the directories being walked
	// would also disturb the concurrent directory listing.
	testFiles, helpers, err := t.scan(ctx)
	if err != nil {
		return err
	}

	// Rewritten contents keyed by file path.
	var changes sync.Map

	if err := t.processTestFiles(ctx, testFiles, helpers, &changes); err != nil {
		return err
	}

	// Do not begin the write phase if we were interrupted.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("interrupted before applying changes: %w", err)
	}

	// Write every file now that every rewrite succeeded. This phase runs to
	// completion without checking for cancellation so that the files are not
	// left in a partially rewritten state.
	return applyChanges(&changes)
}

// testFile is a _test.go file to process.
type testFile struct {
	path string
	src  []byte
}

// scan walks t.in and returns the _test.go files to process. It also indexes
// the helper functions, in any .go file of the tree that imports testing,
// that call t.Setenv or t.Chdir on the testing value they are given.
func (t *tparagen) scan(ctx context.Context) ([]testFile, *helperIndex, error) {
	var (
		mu        sync.Mutex
		testFiles []testFile
		builder   helperIndexBuilder
	)

	// walker.Walk() may execute concurrently.
	err := walker.Walk(t.in, func(path string, info fs.FileInfo) error {
		// Abort the scan early on interruption (SIGINT/SIGTERM); nothing has
		// been written at this point.
		if err := ctx.Err(); err != nil {
			return err
		}

		if info.IsDir() && t.skipDir(path) {
			return filepath.SkipDir
		}

		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}

		isTest := strings.HasSuffix(filepath.Base(path), "_test.go")

		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("cannot read %s. %w", path, err)
		}

		if isTest {
			mu.Lock()
			testFiles = append(testFiles, testFile{path: path, src: src})
			mu.Unlock()
		}

		// Only a file importing testing can declare a helper taking a
		// *testing.T.
		if !bytes.Contains(src, []byte(`"testing"`)) {
			return nil
		}

		f, err := parser.ParseFile(token.NewFileSet(), path, src, parser.SkipObjectResolution)
		if err != nil {
			// A test file that does not parse is reported when it is
			// processed; any other file is not ours to complain about.
			return nil
		}

		builder.addFile(path, f, t.importPath(filepath.Dir(path)))

		return nil
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, nil, fmt.Errorf("interrupted before applying changes: %w", err)
		}

		return nil, nil, fmt.Errorf("error occurred in walker.Walk(). %w", err)
	}

	return testFiles, builder.build(), nil
}

// importPath returns the import path of the package in dir, or "" if dir is
// not in a module.
func (t *tparagen) importPath(dir string) string {
	mod, err := t.modules.find(dir)
	if err != nil || mod == nil || mod.path == "" {
		return ""
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}

	rel, err := filepath.Rel(mod.dir, abs)
	if err != nil {
		return ""
	}

	if rel == "." {
		return mod.path
	}

	return mod.path + "/" + filepath.ToSlash(rel)
}

// processTestFiles rewrites files concurrently and stores the contents that
// changed in changes, keyed by file path. It stops at the first error.
func (t *tparagen) processTestFiles(ctx context.Context, files []testFile, helpers *helperIndex, changes *sync.Map) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	var (
		wg   sync.WaitGroup
		jobs = make(chan testFile)
	)

	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			for file := range jobs {
				if err := t.processTestFile(file, helpers, changes); err != nil {
					cancel(err)
				}
			}
		})
	}

	for _, file := range files {
		if ctx.Err() != nil {
			break
		}

		jobs <- file
	}

	close(jobs)
	wg.Wait()

	if err := context.Cause(ctx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("interrupted before applying changes: %w", err)
		}

		return err
	}

	return nil
}

// processTestFile rewrites one file and stores its new content in changes if
// it changed.
func (t *tparagen) processTestFile(file testFile, helpers *helperIndex, changes *sync.Map) error {
	needFix, err := t.needFixLoopVarIn(filepath.Dir(file.path))
	if err != nil {
		return err
	}

	got, err := generateTParallel(file.path, file.src, needFix, helpers)
	if err != nil {
		return fmt.Errorf("error occurred in Process(). %w", err)
	}

	if !bytes.Equal(file.src, got) {
		changes.Store(file.path, got)
	}

	return nil
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
