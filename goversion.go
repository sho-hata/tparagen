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
	"sync"
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

// needFixLoopVarIn decides whether loop variables must be copied before
// parallel subtests in the files of dir. t.minGoVersion (e.g. "1.21") wins
// when given; otherwise the go directive of the nearest go.mod above dir is
// used, so every module of a multi-module repository gets its own version.
func (t *tparagen) needFixLoopVarIn(dir string) (bool, error) {
	if t.minGoVersion != "" {
		return needFixLoopVar(t.minGoVersion)
	}

	mod, err := t.modules.find(dir)
	if err != nil {
		return false, fmt.Errorf("cannot read go.mod. %w", err)
	}

	if mod == nil {
		t.warnNoGoMod.Do(func() {
			fmt.Fprintf(t.errStream, "go.mod not found: assuming Go %s, so loop variables are copied before parallel subtests (use --min-go-version to override)\n", fallbackGoVersion)
		})

		return needFixLoopVar(fallbackGoVersion)
	}

	needFix, err := needFixLoopVar(mod.goVersion)
	if err != nil {
		return false, fmt.Errorf("%s: %w", filepath.Join(mod.dir, "go.mod"), err)
	}

	return needFix, nil
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

// goModule describes a go.mod file.
type goModule struct {
	// dir is the directory that contains the go.mod.
	dir string
	// path is the module path, from the module directive.
	path string
	// goVersion is the go directive, or noGoDirectiveVersion.
	goVersion string
}

// moduleFinder finds the nearest go.mod of a directory, remembering every
// directory it has looked at. It is safe for concurrent use.
type moduleFinder struct {
	mu    sync.Mutex
	byDir map[string]*goModule // nil when the directory is in no module
}

// find returns the go.mod found in dir or one of its parents, or nil when
// there is none.
func (m *moduleFinder) find(dir string) (*goModule, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.byDir == nil {
		m.byDir = map[string]*goModule{}
	}

	// The directories looked at before finding the go.mod (or the root).
	var visited []string

	var mod *goModule

	for {
		if cached, ok := m.byDir[dir]; ok {
			mod = cached

			break
		}

		visited = append(visited, dir)

		src, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			mod = &goModule{dir: dir, path: parseModulePath(src), goVersion: parseGoDirective(src)}

			break
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	for _, d := range visited {
		m.byDir[d] = mod
	}

	return mod, nil
}

// goVersionFromGoMod returns the go directive of the nearest go.mod found in
// dir or one of its parents. ok is false when there is no go.mod at all.
func goVersionFromGoMod(dir string) (v string, ok bool, err error) {
	mod, err := new(moduleFinder).find(dir)
	if err != nil || mod == nil {
		return "", false, err
	}

	return mod.goVersion, true, nil
}

// goModDirective returns the arguments of the first `<name> ...` directive of
// a go.mod, ignoring comments, or nil if there is none.
func goModDirective(src []byte, name string) []string {
	sc := bufio.NewScanner(bytes.NewReader(src))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}

		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == name {
			return fields[1:]
		}
	}

	return nil
}

// parseGoDirective extracts the version from the `go 1.xx` line of a go.mod.
// A go.mod without a go directive is treated as noGoDirectiveVersion, like the
// go command does.
func parseGoDirective(src []byte) string {
	if args := goModDirective(src, "go"); len(args) == 1 {
		return args[0]
	}

	return noGoDirectiveVersion
}

// parseModulePath extracts the module path from the `module` line of a
// go.mod, or returns "" if there is none.
func parseModulePath(src []byte) string {
	if args := goModDirective(src, "module"); len(args) == 1 {
		return strings.Trim(args[0], `"`)
	}

	return ""
}
