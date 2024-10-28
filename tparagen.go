package tparagen

import (
	"bytes"
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
func Run(outStream, errStream io.Writer, ignoreDirectories []string, minGoVersion float64) error {
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

	return t.run()
}

type tparagen struct {
	in, dest             string
	outStream, errStream io.Writer
	ignoreDirs           []string
	needFixLoopVar       bool
}

// run() traverses from the root node, and when it finds the target test file, it will process the assignment of a concurrency marker.
// The contents of each processed file are written to a temporary file.
// After all scans are complete, rewrite the original file with the contents of each temporary file.
func (t *tparagen) run() error {
	// Information of files to be modified
	// key: original file path, value: temporary file path
	// walker.Walk() may execute concurrently, so sync.Map is used.
	var tempFiles sync.Map

	err := walker.Walk(t.in, func(path string, info fs.FileInfo) error {
		if info.IsDir() && t.skipDir(path) {
			return filepath.SkipDir
		}

		if info.IsDir() || filepath.Ext(path) != ".go" || !strings.HasSuffix(filepath.Base(path), "_test.go") {
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
		tempFiles.Store(path, tmpf.Name())

		b, err := io.ReadAll(f)
		if err != nil {
			return fmt.Errorf("cannot read %s. %w", path, err)
		}

		got, err := GenerateTParallel(path, b, t.needFixLoopVar)
		if err != nil {
			return fmt.Errorf("error occurred in Process(). %w", err)
		}

		if !bytes.Equal(b, got) {
			if _, err := f.WriteAt(got, 0); err != nil {
				return fmt.Errorf("error occurred in writeAt(). %w", err)
			}
		}

		return nil
	})
	// If an error occurs, remove all temporary files
	if err != nil {
		tempFiles.Range(func(_, p any) bool {
			path, ok := p.(string)
			if !ok {
				return false
			}

			// Remove temporary files
			os.Remove(path)

			return true
		})

		return err
	}

	// Replace the original file with the temporary file if all writes are successful
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
			t.errStream.Write([]byte(fmt.Sprintf("failed to rename %s to %s. %v\n", tmpPath, origPath, err)))
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
