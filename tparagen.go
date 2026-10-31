package tparagen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/go/packages"
)

const (
	defaultPattern   = "./..."
	defaultIgnoreDir = "testdata"
)

// Options configures RunWithOptions.
type Options struct {
	// Patterns are the packages to process, in the form the go command
	// accepts (e.g. "./...", "./foo/...", "example.com/m/bar"). When it is
	// empty, every package in and below the current directory ("./...") is
	// processed.
	Patterns []string

	// IgnoreDirs are directory names whose files are left alone, in addition
	// to testdata.
	IgnoreDirs []string

	// MinGoVersion is the Go language version the processed tests are written
	// for (e.g. "1.21"). It decides whether loop variables have to be copied
	// before a parallel subtest, which is needed before Go 1.22. When it is
	// empty, the go directive of the go.mod nearest to each file is used.
	MinGoVersion string

	// BuildTags are the build tags the packages are resolved with, like the
	// -tags flag of the go command. Test files excluded by build constraints
	// are processed anyway when their package is matched, but a directory
	// whose files all need a tag (e.g. //go:build e2e) is matched only when
	// the tag is given.
	BuildTags []string
}

// Run is entry point. It processes every package in and below the current
// directory ("./..."); use RunWithOptions to choose the packages.
//
// minGoVersion is the Go language version the processed tests are written for
// (e.g. "1.21"). It decides whether loop variables have to be copied before a
// parallel subtest, which is needed before Go 1.22. When it is empty, the go
// directive of the go.mod nearest to each file is used.
func Run(ctx context.Context, outStream, errStream io.Writer, ignoreDirectories []string, minGoVersion string) error {
	return RunWithOptions(ctx, outStream, errStream, Options{
		IgnoreDirs:   ignoreDirectories,
		MinGoVersion: minGoVersion,
	})
}

// RunWithOptions processes the packages selected by opts.
func RunWithOptions(ctx context.Context, outStream, errStream io.Writer, opts Options) error {
	ignoreDirs := []string{defaultIgnoreDir}
	for _, dir := range opts.IgnoreDirs {
		if dir != "" {
			ignoreDirs = append(ignoreDirs, dir)
		}
	}

	patterns := opts.Patterns
	if len(patterns) == 0 {
		patterns = []string{defaultPattern}
	}

	// Reject an invalid version before looking at any file.
	if opts.MinGoVersion != "" {
		if _, err := needFixLoopVar(opts.MinGoVersion); err != nil {
			return err
		}
	}

	t := &tparagen{
		patterns:     patterns,
		buildTags:    slices.DeleteFunc(slices.Clone(opts.BuildTags), func(tag string) bool { return tag == "" }),
		outStream:    outStream,
		errStream:    errStream,
		ignoreDirs:   ignoreDirs,
		minGoVersion: opts.MinGoVersion,
	}

	return t.run(ctx)
}

type tparagen struct {
	// dir is the directory the patterns are resolved in; the current
	// directory when empty.
	dir                  string
	patterns             []string
	buildTags            []string
	outStream, errStream io.Writer
	ignoreDirs           []string

	// minGoVersion overrides the go directive of every go.mod when set.
	minGoVersion string
	modules      moduleFinder
	warnNoGoMod  sync.Once
}

func (t *tparagen) run(ctx context.Context) error {
	paths, err := t.testFiles(ctx)
	if err != nil {
		return err
	}

	// Rewritten contents keyed by file path. The run is read-only until every
	// file has been processed successfully: files are written only at the
	// end, so a failure half-way leaves every original untouched. The files
	// are processed concurrently, so sync.Map is used.
	var changes sync.Map

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(runtime.GOMAXPROCS(0))

	for _, path := range paths {
		g.Go(func() error {
			// Abort early on interruption (SIGINT/SIGTERM); nothing has been
			// written at this point.
			if err := gctx.Err(); err != nil {
				return err
			}

			src, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("cannot read %s. %w", path, err)
			}

			needFix, err := t.needFixLoopVarIn(filepath.Dir(path))
			if err != nil {
				return err
			}

			got, err := GenerateTParallel(path, src, needFix)
			if err != nil {
				return fmt.Errorf("error occurred in Process(). %w", err)
			}

			if !bytes.Equal(src, got) {
				changes.Store(path, got)
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("interrupted before applying changes: %w", err)
		}

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

// testFiles resolves t.patterns with go/packages, like `go test` does, and
// returns the _test.go files of the matched packages. Only the directories
// that the patterns select are read: testdata, vendor and directories
// starting with "." or "_" are skipped, and "./..." stops at nested modules.
//
// The packages are only located, not loaded with their tests: go list cannot
// skip resolving imports (-find) when it lists tests, which makes it much
// slower. The _test.go files are read from each package directory instead,
// so test files excluded by build constraints (e.g. //go:build integration)
// are processed as well, whatever tags they need.
func (t *tparagen) testFiles(ctx context.Context) ([]string, error) {
	cfg := &packages.Config{
		Context: ctx,
		Mode:    packages.NeedName | packages.NeedFiles,
		Dir:     t.dir,
	}

	if len(t.buildTags) > 0 {
		cfg.BuildFlags = []string{"-tags=" + strings.Join(t.buildTags, ",")}
	}

	pkgs, err := packages.Load(cfg, t.patterns...)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("interrupted before applying changes: %w", ctxErr)
		}

		return nil, fmt.Errorf("cannot load packages. %w", err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages matched %s", strings.Join(t.patterns, " "))
	}

	baseDir, err := filepath.Abs(t.dir)
	if err != nil {
		return nil, err
	}

	var (
		paths   []string
		seen    = map[string]bool{}
		loadErr []error
	)

	for _, pkg := range pkgs {
		// A pattern that matches nothing (a missing directory, a package
		// outside the main modules, ...) is reported as a package without a
		// directory. Errors of packages that do have one, such as a directory
		// holding only test files, do not keep us from rewriting their tests.
		if pkg.Dir == "" {
			for _, e := range pkg.Errors {
				loadErr = append(loadErr, errors.New(e.Error()))
			}

			continue
		}

		if seen[pkg.Dir] || t.ignored(baseDir, pkg.Dir) {
			continue
		}

		seen[pkg.Dir] = true

		entries, err := os.ReadDir(pkg.Dir)
		if err != nil {
			return nil, fmt.Errorf("cannot read %s. %w", pkg.Dir, err)
		}

		for _, e := range entries {
			if e.Type().IsRegular() && strings.HasSuffix(e.Name(), "_test.go") {
				paths = append(paths, filepath.Join(pkg.Dir, e.Name()))
			}
		}
	}

	if len(loadErr) > 0 {
		return nil, fmt.Errorf("cannot load packages. %w", errors.Join(loadErr...))
	}

	return paths, nil
}

// ignored reports whether dir is, or is in, a directory named by
// t.ignoreDirs, below baseDir (or anywhere in dir when it is outside baseDir).
func (t *tparagen) ignored(baseDir, dir string) bool {
	rel, err := filepath.Rel(baseDir, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		rel = dir
	}

	for name := range strings.SplitSeq(rel, string(filepath.Separator)) {
		if name != "" && slices.Contains(t.ignoreDirs, name) {
			return true
		}
	}

	return false
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
