package tparagen

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A test that hands its *testing.T to a helper calling t.Setenv or t.Chdir on
// it panics once it is parallel, just like one that calls them itself.
func TestGenerateTParallelHelpers(t *testing.T) {
	t.Parallel()

	runGolden(t, "helpers")
}

// Run finds helpers declared in other files of the package and in other
// packages of the tree, including non-test files such as a testutil package.
func TestRunHelpersInOtherFilesAndPackages(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	files := map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.26\n",
		"internal/testutil/mktemp.go": `package testutil

import "testing"

func Mktmp(tb testing.TB) string {
	dir := tb.TempDir()
	tb.Chdir(dir)
	return dir
}
`,
		"internal/testutil/other.go": `package testutil

func Mktmp2() {}
`,
		"a/helper_test.go": `package a

import "testing"

func setEnv(t *testing.T) {
	t.Setenv("A", "1")
}
`,
		"a/a_test.go": `package a

import (
	"testing"

	tu "example.com/m/internal/testutil"
)

func TestImported(t *testing.T) {
	tu.Mktmp(t)
}

func TestSamePackage(t *testing.T) {
	setEnv(t)
}

func TestNeither(t *testing.T) {
	tu.Mktmp2()
}
`,
	}

	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := newRunner(root).run(context.Background()); err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "a", "a_test.go"))
	if err != nil {
		t.Fatal(err)
	}

	const want = `package a

import (
	"testing"

	tu "example.com/m/internal/testutil"
)

func TestImported(t *testing.T) {
	tu.Mktmp(t)
}

func TestSamePackage(t *testing.T) {
	setEnv(t)
}

func TestNeither(t *testing.T) {
	t.Parallel()
	tu.Mktmp2()
}
`

	if string(got) != want {
		t.Errorf("result:\n%s\nwant:\n%s", got, want)
	}
}
