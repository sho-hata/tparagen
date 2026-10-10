package tparagen

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/version"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	// loopVarFixVersion is the first Go language version in which every
	// iteration of a for loop has its own copy of the loop variable.
	// https://go.dev/doc/go1.22#language
	loopVarFixVersion = "go1.22"

	// fallbackGoVersion is assumed when no go.mod can be found and no version
	// was given explicitly. It predates loopVarFixVersion, so loop variables
	// are copied, which is always safe (merely redundant on newer versions).
	fallbackGoVersion = "1.21"

	// noGoDirectiveVersion is what the go command assumes for a go.mod that
	// has no go directive.
	noGoDirectiveVersion = "1.16"
)

// resolveNeedFixLoopVar decides whether loop variables must be copied before
// parallel subtests. minGoVersion (e.g. "1.21") wins when given; otherwise the
// go directive of the nearest go.mod above t.in is used.
func (t *tparagen) resolveNeedFixLoopVar(minGoVersion string) (bool, error) {
	v := minGoVersion

	if v == "" {
		detected, ok, err := goVersionFromGoMod(t.in)
		if err != nil {
			return false, fmt.Errorf("cannot read go.mod. %w", err)
		}

		if ok {
			v = detected
		} else {
			v = fallbackGoVersion

			fmt.Fprintf(t.errStream, "go.mod not found: assuming Go %s, so loop variables are copied before parallel subtests (use --min-go-version to override)\n", v)
		}
	}

	return needFixLoopVar(v)
}

// needFixLoopVar reports whether code written for Go language version v (e.g.
// "1.21", "1.22.4" or "go1.21") still shares the loop variable between
// iterations and therefore needs the `tc := tc` copy before a parallel subtest.
func needFixLoopVar(v string) (bool, error) {
	normalized := strings.TrimSpace(v)
	if !strings.HasPrefix(normalized, "go") {
		normalized = "go" + normalized
	}

	if !version.IsValid(normalized) {
		return false, fmt.Errorf("invalid go version %q (expected e.g. \"1.21\")", v)
	}

	return version.Compare(normalized, loopVarFixVersion) < 0, nil
}

// goVersionFromGoMod returns the go directive of the nearest go.mod found in
// dir or one of its parents. ok is false when there is no go.mod at all.
func goVersionFromGoMod(dir string) (v string, ok bool, err error) {
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", false, err
	}

	for {
		src, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			return parseGoDirective(src), true, nil
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return "", false, err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}

		dir = parent
	}
}

// parseGoDirective extracts the version from the `go 1.xx` line of a go.mod.
// A go.mod without a go directive is treated as noGoDirectiveVersion, like the
// go command does.
func parseGoDirective(src []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(src))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}

		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			return fields[1]
		}
	}

	return noGoDirectiveVersion
}
