package tparagen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Helpers that a test calls through less common forms of call or import still
// keep the test sequential: each case panicked with "can not use t.Parallel"
// before.
func TestRunHelpersReachedIndirectly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase string
		files    map[string]string
		testFile string
	}{
		{
			testCase: "package name differs from the directory",
			files: map[string]string{
				"go.mod": `module example.com/m

go 1.24
`,
				"p/p_test.go": `package p

import (
	"testing"

	"example.com/m/test-util"
)

func TestX(t *testing.T) {
	testutil.SetHome(t)
}
`,
				"test-util/u.go": `package testutil

import "testing"

func SetHome(t *testing.T) { t.Setenv("HOME", "/x") }
`,
			},
			testFile: "p/p_test.go",
		},
		{
			testCase: "dot import",
			files: map[string]string{
				"go.mod": `module example.com/m

go 1.24
`,
				"p/p_test.go": `package p

import (
	"testing"

	. "example.com/m/tu"
)

func TestX(t *testing.T) {
	SetHome(t)
}
`,
				"tu/u.go": `package tu

import "testing"

func SetHome(t *testing.T) { t.Setenv("HOME", "/x") }
`,
			},
			testFile: "p/p_test.go",
		},
		{
			testCase: "generic helper called with type arguments",
			files: map[string]string{
				"go.mod": `module example.com/m

go 1.24
`,
				"p_test.go": `package p

import "testing"

func setVal[V any](t *testing.T, v V) { t.Setenv("K", "v") }

func TestX(t *testing.T) {
	setVal[int](t, 1)
}
`,
			},
			testFile: "p_test.go",
		},
		{
			testCase: "method on a nested field",
			files: map[string]string{
				"go.mod": `module example.com/m

go 1.24
`,
				"p_test.go": `package p

import "testing"

type env struct{}

func (env) setup(t *testing.T) { t.Setenv("K", "v") }

type suite struct{ e env }

func TestX(t *testing.T) {
	s := suite{}
	s.e.setup(t)
}
`,
			},
			testFile: "p_test.go",
		},
		{
			testCase: "vendored helper",
			files: map[string]string{
				"go.mod": `module example.com/m

go 1.24

require example.com/tu v0.0.0
`,
				"p/p_test.go": `package p

import (
	"testing"

	"example.com/tu"
)

func TestX(t *testing.T) {
	tu.SetHome(t)
}
`,
				"vendor/example.com/tu/u.go": `package tu

import "testing"

func SetHome(t *testing.T) { t.Setenv("HOME", "/x") }
`,
				"vendor/modules.txt": `# example.com/tu v0.0.0
## explicit
example.com/tu
`,
			},
			testFile: "p/p_test.go",
		},
		{
			testCase: "helper importing testing under another name",
			files: map[string]string{
				"go.mod": `module example.com/m

go 1.24
`,
				"p/p_test.go": `package p

import (
	"testing"

	"example.com/m/tu"
)

func TestX(t *testing.T) {
	tu.SetHome(t)
}
`,
				"tu/u.go": `package tu

import tst "testing"

func SetHome(t *tst.T) { t.Setenv("HOME", "/x") }
`,
			},
			testFile: "p/p_test.go",
		},
		{
			testCase: "method of a type from another package",
			files: map[string]string{
				"go.mod": `module example.com/m

go 1.24
`,
				"p/p_test.go": `package p

import (
	"testing"

	"example.com/m/tu"
)

func TestX(t *testing.T) {
	var e tu.Env
	e.SetHome(t)
}
`,
				"tu/u.go": `package tu

import "testing"

type Env struct{}

func (Env) SetHome(t *testing.T) { t.Setenv("HOME", "/x") }
`,
			},
			testFile: "p/p_test.go",
		},
		{
			testCase: "package name differs from the import path",
			files: map[string]string{
				"go.mod": `module example.com/m

go 1.24
`,
				"helpers/u.go": `package th

import "testing"

func SetHome(t *testing.T) { t.Setenv("HOME", "/x") }
`,
				"p/p_test.go": `package p

import (
	"testing"

	"example.com/m/helpers"
)

func TestX(t *testing.T) {
	th.SetHome(t)
}
`,
			},
			testFile: "p/p_test.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testCase, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()

			for name, content := range tt.files {
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

			got, err := os.ReadFile(filepath.Join(root, tt.testFile))
			if err != nil {
				t.Fatal(err)
			}

			if strings.Contains(string(got), "Parallel()") {
				t.Errorf("t.Parallel() was inserted:\n%s", got)
			}
		})
	}
}
