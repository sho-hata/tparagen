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
	// Information of files to be modified
	// key: original file path, value: temporary file path
	// walker.Walk() may execute concurrently, so sync.Map is used.
	var tempFiles sync.Map

	// remove all temporary files
	defer func() {
		tempFiles.Range(func(_, p any) bool {
			path, ok := p.(string)
			if !ok {
				return false
			}

			// Remove temporary files
			os.Remove(path)

			return true
		})
	}()

	if err := walker.Walk(t.in, func(path string, info fs.FileInfo) error {
		// Abort the scan early on interruption (SIGINT/SIGTERM). Returning here
		// lets the deferred cleanup remove any temporary files created so far.
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

		f, err := os.OpenFile(path, os.O_RDWR, 0664)
		if err != nil {
			return fmt.Errorf("cannot open %s. %w", path, err)
		}
		defer f.Close()

		tmpf, err := os.CreateTemp("", "temp_")
		if err != nil {
			return fmt.Errorf("failed to create temp file for %s. %w", path, err)
		}

		defer tmpf.Close()

		b, err := io.ReadAll(f)
		if err != nil {
			return fmt.Errorf("cannot read %s. %w", path, err)
		}

		got, err := GenerateTParallel(path, b, t.needFixLoopVar)
		if err != nil {
			return fmt.Errorf("error occurred in Process(). %w", err)
		}

		if !bytes.Equal(b, got) {
			if _, err := tmpf.WriteAt(got, 0); err != nil {
				return fmt.Errorf("error occurred in writeAt(). %w", err)
			}
			tempFiles.Store(path, tmpf.Name())
		}

		return nil
	}); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("interrupted before applying changes: %w", err)
		}

		return fmt.Errorf("error occurred in walker.Walk(). %w", err)
	}

	// Do not begin the destructive rename phase if we were interrupted during
	// the scan. The deferred cleanup removes the temporary files, leaving the
	// original files untouched.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("interrupted before applying changes: %w", err)
	}

	// Replace the original file with the temporary file if all writes are successful.
	// This phase runs to completion without checking for cancellation so that the
	// files are not left in a partially rewritten state.
	tempFiles.Range(func(key, value any) bool {
		origPath, ok := key.(string)
		if !ok {
			return false
		}

		tmpPath, ok := value.(string)
		if !ok {
			return false
		}

		if err := os.Rename(tmpPath, origPath); err != nil {
			// TODO: logging
			if _, err := fmt.Fprintf(t.errStream, "failed to rename %s to %s. %v\n", tmpPath, origPath, err); err != nil {
				return false
			}
		}

		return true
	})

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
