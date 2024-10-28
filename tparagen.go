package tparagen

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

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

func (t *tparagen) run() error {
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
		b, err := io.ReadAll(f)
		if err != nil {
			return fmt.Errorf("cannot read %s. %w", path, err)
		}

		got, err := Process(path, b, t.needFixLoopVar)
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

	return err
}

func (t *tparagen) skipDir(p string) bool {
	for _, dir := range t.ignoreDirs {
		if filepath.Base(p) == dir {
			return true
		}
	}

	return false
}
