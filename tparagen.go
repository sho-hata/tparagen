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
	return walker.Walk(t.in, func(path string, info fs.FileInfo) error {
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
		b, err := io.ReadAll(f)
		if err != nil {
			return fmt.Errorf("cannot read %s. %w", path, err)
		}

		got, err := Process(path, b, t.needFixLoopVar)
		if err != nil {
			return fmt.Errorf("error occurred in Process(). %w", err)
		}

		if !bytes.Equal(b, got) {
			if len(t.dest) != 0 && t.in != t.dest {
				if err := t.writeOtherPath(t.in, t.dest, path, got); err != nil {
					return fmt.Errorf("error occurred in triteOtherPath(). %w", err)
				}
			}
			if _, err := f.WriteAt(got, 0); err != nil {
				return fmt.Errorf("error occurred in writeAt(). %w", err)
			}
		}

		return nil
	})
}

func (t *tparagen) writeOtherPath(in, dist, path string, got []byte) error {
	p, err := filepath.Rel(in, path)
	if err != nil {
		return err
	}

	distabs, err := filepath.Abs(dist)
	if err != nil {
		return err
	}

	dp := filepath.Join(distabs, p)
	dpd := filepath.Dir(dp)

	if _, err := os.Stat(dpd); os.IsNotExist(err) {
		if err := os.Mkdir(dpd, 0777); err != nil {
			return fmt.Errorf("create dir failed at %q: %w", dpd, err)
		}
	}

	f, err := os.OpenFile(dp, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil
	}
	defer f.Close()

	if _, err = f.Write(got); err != nil {
		return fmt.Errorf("write file failed at %q: %w", dp, err)
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
