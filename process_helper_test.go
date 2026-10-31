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

	tests := []struct {
		testCase string
		src      string
		want     string
	}{
		{
			testCase: "a helper calling Setenv keeps the test and its ancestors sequential",
			src: `package t

import "testing"

func setEnv(t *testing.T) {
	t.Setenv("A", "1")
}

func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		setEnv(t)
	})
	t.Run("2", func(t *testing.T) {
		fmt.Println("2")
	})
}
`,
			want: `package t

import "testing"

func setEnv(t *testing.T) {
	t.Setenv("A", "1")
}

func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		setEnv(t)
	})
	t.Run("2", func(t *testing.T) {
		t.Parallel()
		fmt.Println("2")
	})
}
`,
		},
		{
			testCase: "a chain of helpers taking testing.TB counts",
			src: `package t

import "testing"

func mktmp(tb testing.TB) string {
	dir := tb.TempDir()
	chdir(tb, dir)
	return dir
}

func chdir(tb testing.TB, dir string) {
	tb.Chdir(dir)
}

func TestX(t *testing.T) {
	mktmp(t)
}
`,
			want: `package t

import "testing"

func mktmp(tb testing.TB) string {
	dir := tb.TempDir()
	chdir(tb, dir)
	return dir
}

func chdir(tb testing.TB, dir string) {
	tb.Chdir(dir)
}

func TestX(t *testing.T) {
	mktmp(t)
}
`,
		},
		{
			testCase: "a helper calling Setenv in a subtest of its own counts",
			src: `package t

import "testing"

func runWithEnv(t *testing.T) {
	t.Run("env", func(t *testing.T) {
		t.Setenv("A", "1")
	})
}

func TestX(t *testing.T) {
	runWithEnv(t)
}
`,
			want: `package t

import "testing"

func runWithEnv(t *testing.T) {
	t.Run("env", func(t *testing.T) {
		t.Setenv("A", "1")
	})
}

func TestX(t *testing.T) {
	runWithEnv(t)
}
`,
		},
		{
			testCase: "a method helper counts",
			src: `package t

import "testing"

type env struct{}

func (e *env) setup(t *testing.T) {
	t.Setenv("A", "1")
}

func TestX(t *testing.T) {
	e := &env{}
	e.setup(t)
}
`,
			want: `package t

import "testing"

type env struct{}

func (e *env) setup(t *testing.T) {
	t.Setenv("A", "1")
}

func TestX(t *testing.T) {
	e := &env{}
	e.setup(t)
}
`,
		},
		{
			testCase: "a helper closure taking testing.TB counts",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	doTest := func(tb testing.TB) {
		testlib.Mktmp(tb)
		tb.Chdir("x")
	}

	t.Run("1", func(t *testing.T) {
		doTest(t)
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	doTest := func(tb testing.TB) {
		testlib.Mktmp(tb)
		tb.Chdir("x")
	}

	t.Run("1", func(t *testing.T) {
		doTest(t)
	})
}
`,
		},
		{
			testCase: "a helper that does not call Setenv or Chdir does not matter",
			src: `package t

import "testing"

func check(t *testing.T, got int) {
	t.Helper()
	if got != 1 {
		t.Fatal(got)
	}
}

func TestX(t *testing.T) {
	check(t, 1)
}
`,
			want: `package t

import "testing"

func check(t *testing.T, got int) {
	t.Helper()
	if got != 1 {
		t.Fatal(got)
	}
}

func TestX(t *testing.T) {
	t.Parallel()
	check(t, 1)
}
`,
		},
		{
			testCase: "calling a helper without the testing value does not matter",
			src: `package t

import "testing"

func setEnv(t *testing.T) {
	t.Setenv("A", "1")
}

func TestX(t *testing.T) {
	f := setEnv
	_ = f
}
`,
			want: `package t

import "testing"

func setEnv(t *testing.T) {
	t.Setenv("A", "1")
}

func TestX(t *testing.T) {
	t.Parallel()
	f := setEnv
	_ = f
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testCase, func(t *testing.T) {
			t.Parallel()

			got, err := GenerateTParallel("./testdata/t/t_test.go", []byte(tt.src), false)
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != tt.want {
				t.Errorf("result:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
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
