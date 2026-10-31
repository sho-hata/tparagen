// Package benchcorpus generates deterministic Go test files for benchmarking
// tparagen.
package benchcorpus

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
)

// Spec describes the shape of a generated corpus.
type Spec struct {
	Packages        int
	FilesPerPackage int
	TestsPerFile    int
	// NestDepth is the depth of nested t.Run calls in the "nested" test shape.
	NestDepth int
	// GoVersion is written to go.mod. Versions before 1.22 make tparagen copy
	// loop variables.
	GoVersion string
}

// shapes is cycled through, so every file of 7 or more tests exercises each
// rewrite path of tparagen. Each template receives a name that is unique within
// its package and the nesting depth.
var shapes = []func(name string, depth int) string{
	func(name string, _ int) string {
		return fmt.Sprintf(`func Test%s(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"one", 1, 2},
		{"two", 2, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in * 2; got != tt.want {
				t.Errorf("got %%d, want %%d", got, tt.want)
			}
		})
	}
}
`, name)
	},
	func(name string, _ int) string {
		return fmt.Sprintf(`func Test%s(t *testing.T) {
	names := []string{"a", "b"}
	for i, n := range names {
		t.Run(n, func(t *testing.T) {
			_ = i
		})
	}
	for _, n := range names {
		t.Run(n+"2", func(t *testing.T) {
			_ = len(n)
		})
	}
}
`, name)
	},
	func(name string, _ int) string {
		return fmt.Sprintf(`func Test%s(t *testing.T) {
	enabled := len(t.Name()) > 0
	if enabled {
		t.Run("enabled", func(t *testing.T) {
			_ = enabled
		})
	}
}
`, name)
	},
	func(name string, _ int) string {
		return fmt.Sprintf(`func Test%s(t *testing.T) {
	for i := 0; i < 3; i++ {
		t.Run("loop", func(t *testing.T) {
			_ = i
		})
	}
}
`, name)
	},
	nested,
	func(name string, _ int) string {
		return fmt.Sprintf(`func Test%s(t *testing.T) {
	t.Setenv("TPARAGEN_BENCH", "1")
	t.Run("env", func(t *testing.T) {})
}
`, name)
	},
	func(name string, _ int) string {
		return fmt.Sprintf(`func Test%s(t *testing.T) {
	t.Parallel()
	t.Run("already", func(t *testing.T) {})
}
`, name)
	},
}

// nested builds depth levels of t.Run, each inside its own for loop, with every
// loop variable used in the innermost body so the file compiles. Each loop has
// one element so that running the generated tests stays linear in depth.
func nested(name string, depth int) string {
	var b strings.Builder

	fmt.Fprintf(&b, "func Test%s(t *testing.T) {\n", name)

	uses := make([]string, depth)
	for i := range depth {
		uses[i] = fmt.Sprintf("v%d", i)
		fmt.Fprintf(&b, "for _, v%d := range []int{1} {\nt.Run(\"n%d\", func(t *testing.T) {\n", i, i)
	}

	if depth > 0 {
		fmt.Fprintf(&b, "_ = %s\n", strings.Join(uses, " + "))
	}

	for range depth {
		b.WriteString("})\n}\n")
	}

	b.WriteString("}\n")

	return b.String()
}

// File returns the source of the file-th test file of package pkg. The output
// depends only on its arguments and is gofmt-clean.
func File(spec Spec, pkg, file int) []byte {
	var b strings.Builder

	fmt.Fprintf(&b, "package p%d\n\nimport \"testing\"\n", pkg)

	for k := range spec.TestsPerFile {
		b.WriteString("\n")
		b.WriteString(shapes[k%len(shapes)](fmt.Sprintf("F%dT%d", file, k), spec.NestDepth))
	}

	src, err := format.Source([]byte(b.String()))
	if err != nil {
		// A template bug, not a runtime condition.
		panic(fmt.Sprintf("benchcorpus: generated source does not parse: %v", err))
	}

	return src
}

// Write creates a module in dir with Spec.Packages package directories, each
// holding Spec.FilesPerPackage test files. Spreading files over directories
// matters: tparagen walks directories concurrently, so one flat directory
// would run on a single core.
func Write(dir string, spec Spec) error {
	goMod := fmt.Sprintf("module benchcorpus\n\ngo %s\n", spec.GoVersion)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		return err
	}

	for p := range spec.Packages {
		pkgDir := filepath.Join(dir, fmt.Sprintf("pkg%03d", p))
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			return err
		}

		for f := range spec.FilesPerPackage {
			path := filepath.Join(pkgDir, fmt.Sprintf("f%03d_test.go", f))
			if err := os.WriteFile(path, File(spec, p, f), 0o644); err != nil {
				return err
			}
		}
	}

	return nil
}
