package tparagen

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// The run ends with a short summary; per-file lines only with verbose.
func TestRunReport(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"a_test.go": rewritableTestSrc,
		"b_test.go": `package t

import "testing"

func TestEnv(t *testing.T) {
	t.Setenv("X", "y")
}
`,
		"c_test.go": `package t

import "testing"

func TestDone(t *testing.T) {
	t.Parallel()
}
`,
	}

	tests := []struct {
		testCase string
		verbose  bool
		want     string
	}{
		{
			testCase: "summary only",
			want: "Updated 1 of 3 test files: 2 t.Parallel() added\n" +
				"1 test left sequential: t.Parallel() is not safe there (t.Setenv, global state, ...). Run with -v to see the files.\n",
		},
		{
			testCase: "verbose",
			verbose:  true,
			want: "a_test.go: 2 t.Parallel() added\n" +
				"b_test.go: 1 test left sequential\n" +
				"Updated 1 of 3 test files: 2 t.Parallel() added\n" +
				"1 test left sequential: t.Parallel() is not safe there (t.Setenv, global state, ...).\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testCase, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for name, src := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			var out bytes.Buffer

			r := &tparagen{
				in:         dir,
				outStream:  &out,
				errStream:  io.Discard,
				ignoreDirs: []string{defaultIgnoreDir},
				verbose:    tt.verbose,
			}
			if err := r.run(context.Background()); err != nil {
				t.Fatal(err)
			}

			if got, want := out.String(), prefixPaths(tt.want, dir+string(filepath.Separator)); got != want {
				t.Errorf("output:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestRunReportNoChanges(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "c_test.go"), []byte("package t\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer

	r := &tparagen{in: dir, outStream: &out, errStream: io.Discard, ignoreDirs: []string{defaultIgnoreDir}}
	if err := r.run(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got, want := out.String(), "No changes in 1 test file\n"; got != want {
		t.Errorf("output: %q, want %q", got, want)
	}
}

// prefixPaths prefixes the file names at the start of the verbose lines of s.
func prefixPaths(s, prefix string) string {
	var b bytes.Buffer

	for _, line := range bytes.SplitAfter([]byte(s), []byte("\n")) {
		if bytes.HasSuffix(bytes.SplitN(line, []byte(":"), 2)[0], []byte("_test.go")) {
			b.WriteString(prefix)
		}

		b.Write(line)
	}

	return b.String()
}
