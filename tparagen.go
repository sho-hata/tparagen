package tparagen

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrFlagTrue = errors.New("find error")
)

// Run is entry point.
func Run(args []string, outStream, errStream io.Writer) error {
	var tparagen *tparagen

	tparagen, err := fill(args, outStream, errStream)
	if err != nil {
		return err
	}

	err = tparagen.run()
	if tparagen.errFlag {
		err = ErrFlagTrue
	}

	return err
}

func fill(args []string, outStream, errStream io.Writer) (*tparagen, error) {
	cn := args[0]
	flags := flag.NewFlagSet(cn, flag.ContinueOnError)
	flags.SetOutput(errStream)
	flags.Usage = func() {
		fmt.Fprintf(
			flag.CommandLine.Output(),
			"tparagen embeds `t.Parallel()` in a test function in a specific source file or in an entire directory.\n\nUsage of %s:\n",
			os.Args[0],
		)
		flags.PrintDefaults()
	}

	var destDir string

	odesc := "destination directory."
	flags.StringVar(&destDir, "destination", "", odesc)

	if err := flags.Parse(args[1:]); err != nil {
		return nil, err
	}

	targetDir := "./"

	nargs := flags.Args()
	if len(nargs) > 1 {
		return nil, errors.New("execution path must be only one or no-set(current directory)")
	}

	if len(nargs) == 1 {
		targetDir = nargs[0]
	}

	return &tparagen{
		in:        targetDir,
		dest:      destDir,
		outStream: outStream,
		errStream: errStream,
	}, nil
}

type tparagen struct {
	in, dest             string
	outStream, errStream io.Writer
	errFlag              bool
}

func (t *tparagen) run() error {
	return filepath.Walk(t.in, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
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
			return err
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			return err
		}

		got, err := Process(path, b)
		if err != nil {
			return err
		}

		if !bytes.Equal(b, got) {
			if len(t.dest) != 0 && t.in != t.dest {
				return t.writeOtherPath(t.in, t.dest, path, got)
			}
			if _, err := f.WriteAt(got, 0); err != nil {
				return err
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
			fmt.Fprintf(t.outStream, "create dir failed at %q: %v\n", dpd, err)

			return err
		}
	}

	fmt.Fprintf(t.outStream, "update file %q\n", dp)

	f, err := os.OpenFile(dp, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil
	}
	defer f.Close()

	if _, err = f.Write(got); err != nil {
		fmt.Fprintf(t.outStream, "write file failed %v\n", err)
	}

	fmt.Printf("created at %q\n", dp)

	return nil
}
