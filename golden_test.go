package tparagen

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"
)

// update rewrites the golden.go section of every golden file with the current
// output: go test -run TestGenerateTParallel -update. Review the diff before
// committing it.
var update = flag.Bool("update", false, "rewrite the golden files in testdata/golden with the current output")

// goldenFilename is the file name GenerateTParallel is given for every case.
const goldenFilename = "./testdata/t/t_test.go"

// goldenCase is one testdata/golden/<dir>/*.txtar file. Its comment holds
// "Key: value" lines:
//
//	Name: the subtest name (none for a directory with a single case)
//	NeedFixLoopVar: true or false, passed to GenerateTParallel
//	NoFinalNewline: input and/or golden, for a section that does not end in
//	  a newline (txtar always ends a section with one)
//
// followed by the sections input.go, the source to rewrite, and golden.go,
// the wanted output.
type goldenCase struct {
	path           string
	archive        *txtar.Archive
	name           string
	needFixLoopVar bool
	noFinalNewline map[string]bool
	input, golden  []byte
}

// runGolden runs GenerateTParallel on every case in testdata/golden/<dir> and
// compares the result with its golden.go section.
func runGolden(t *testing.T, dir string) {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join("testdata", "golden", dir, "*.txtar"))
	if err != nil {
		t.Fatal(err)
	}

	if len(paths) == 0 {
		t.Fatalf("no golden files in testdata/golden/%s", dir)
	}

	for _, path := range paths {
		c := readGoldenCase(t, path)

		if c.name == "" {
			if len(paths) != 1 {
				t.Fatalf("%s: a case without Name must be the only one of its directory", path)
			}

			checkGolden(t, c)

			continue
		}

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			checkGolden(t, c)
		})
	}
}

func checkGolden(t *testing.T, c *goldenCase) {
	t.Helper()

	got, err := GenerateTParallel(goldenFilename, c.input, c.needFixLoopVar)
	if err != nil {
		t.Fatal(err)
	}

	if *update {
		writeGoldenCase(t, c, got)

		return
	}

	if !bytes.Equal(got, c.golden) {
		t.Errorf("%s: result differs from golden.go %s\nresult:\n%s\ngolden.go:\n%s\n(run go test -update to rewrite the golden files)",
			c.path, firstDiff(got, c.golden), got, c.golden)
	}
}

func readGoldenCase(t *testing.T, path string) *goldenCase {
	t.Helper()

	a, err := txtar.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	c := &goldenCase{path: path, archive: a, noFinalNewline: map[string]bool{}}

	for line := range strings.Lines(string(a.Comment)) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("%s: comment line %q is not \"Key: value\"", path, line)
		}

		value = strings.TrimSpace(value)

		switch key {
		case "Name":
			c.name = value
		case "NeedFixLoopVar":
			if c.needFixLoopVar, err = strconv.ParseBool(value); err != nil {
				t.Fatalf("%s: NeedFixLoopVar: %v", path, err)
			}
		case "NoFinalNewline":
			for _, section := range strings.Fields(value) {
				c.noFinalNewline[section] = true
			}
		default:
			t.Fatalf("%s: unknown key %q", path, key)
		}
	}

	for _, f := range a.Files {
		data := f.Data
		if c.noFinalNewline[strings.TrimSuffix(f.Name, ".go")] {
			data = bytes.TrimSuffix(data, []byte("\n"))
		}

		switch f.Name {
		case "input.go":
			c.input = data
		case "golden.go":
			c.golden = data
		default:
			t.Fatalf("%s: unknown section %q", path, f.Name)
		}
	}

	if c.input == nil || c.golden == nil {
		t.Fatalf("%s: input.go and golden.go are both required", path)
	}

	return c
}

// writeGoldenCase replaces the golden.go section of c's file with got.
func writeGoldenCase(t *testing.T, c *goldenCase, got []byte) {
	t.Helper()

	c.noFinalNewline["golden"] = len(got) > 0 && got[len(got)-1] != '\n'

	var comment bytes.Buffer
	if c.name != "" {
		fmt.Fprintf(&comment, "Name: %s\n", c.name)
	}

	fmt.Fprintf(&comment, "NeedFixLoopVar: %t\n", c.needFixLoopVar)

	var noFinalNewline []string

	for _, section := range []string{"input", "golden"} {
		if c.noFinalNewline[section] {
			noFinalNewline = append(noFinalNewline, section)
		}
	}

	if len(noFinalNewline) > 0 {
		fmt.Fprintf(&comment, "NoFinalNewline: %s\n", strings.Join(noFinalNewline, " "))
	}

	a := &txtar.Archive{
		Comment: comment.Bytes(),
		Files: []txtar.File{
			{Name: "input.go", Data: c.input},
			{Name: "golden.go", Data: got},
		},
	}

	if err := os.WriteFile(c.path, txtar.Format(a), 0o644); err != nil {
		t.Fatal(err)
	}
}

// firstDiff describes the first line where got and want differ.
func firstDiff(got, want []byte) string {
	gotLines := strings.Split(string(got), "\n")
	wantLines := strings.Split(string(want), "\n")

	for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
		var g, w string
		if i < len(gotLines) {
			g = gotLines[i]
		}

		if i < len(wantLines) {
			w = wantLines[i]
		}

		if g != w || i >= len(gotLines) || i >= len(wantLines) {
			return fmt.Sprintf("at line %d:\n  result:    %q\n  golden.go: %q", i+1, g, w)
		}
	}

	return "(only in the final newline)"
}
