package tparagen

import "testing"

// Regression tests for nolint handling. hasNolintCommentDirective used to
// return true for every comment group, so any test function with a doc comment
// and any file starting with a comment (build constraint, license header) was
// silently skipped.
func TestGenerateTParallelNolint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase  string
		src       string
		processed bool
	}{
		{
			testCase: "a doc comment on the test function is not a nolint directive",
			src: `package t

import "testing"

// TestX checks x.
func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {})
}
`,
			processed: true,
		},
		{
			testCase: "a build constraint at the top of the file is not a nolint directive",
			src: `//go:build integration

package t

import "testing"

func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {})
}
`,
			processed: true,
		},
		{
			testCase: "a license header is not a nolint directive",
			src: `// Copyright 2024 The Authors. MIT License.

package t

import "testing"

func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {})
}
`,
			processed: true,
		},
		{
			testCase: "a package doc comment is not a nolint directive",
			src: `// Package t does x.
package t

import "testing"

func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {})
}
`,
			processed: true,
		},
		{
			testCase: "nolint for an unrelated linter does not exclude the function",
			src: `package t

import "testing"

//nolint:unused
func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {})
}
`,
			processed: true,
		},
		{
			testCase: "nolint with a trailing explanation excludes the function",
			src: `package t

import "testing"

//nolint:paralleltest // uses a global
func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {})
}
`,
			processed: false,
		},
		{
			testCase: "nolint:all excludes the function",
			src: `package t

import "testing"

//nolint:all
func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {})
}
`,
			processed: false,
		},
		{
			testCase: "nolint on the last line of a doc comment excludes the function",
			src: `package t

import "testing"

// TestX checks x.
//
//nolint:tparallel
func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {})
}
`,
			processed: false,
		},
		{
			testCase: "file-level nolint after a build constraint excludes the file",
			src: `//go:build integration

//nolint:tparallel
package t

import "testing"

func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {})
}
`,
			processed: false,
		},
		{
			testCase: "file-level nolint:all excludes the file",
			src: `//nolint:all
package t

import "testing"

func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {})
}
`,
			processed: false,
		},
		{
			testCase: "a nolint directive after the package clause does not apply to the file",
			src: `package t

import "testing"

//nolint:tparallel
var _ = 1

func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {})
}
`,
			processed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testCase, func(t *testing.T) {
			t.Parallel()

			got, err := GenerateTParallel("./testdata/t/t_test.go", []byte(tt.src), false)
			if err != nil {
				t.Fatal(err)
			}

			if changed := string(got) != tt.src; changed != tt.processed {
				t.Errorf("processed = %v, want %v\nresult:\n%s", changed, tt.processed, got)
			}
		})
	}
}

func TestIsNolintDirectiveForTparagen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		want bool
	}{
		{text: "//nolint", want: true},
		{text: "//nolint // reason", want: true},
		{text: "//nolint:all", want: true},
		{text: "//nolint:tparallel", want: true},
		{text: "//nolint:paralleltest", want: true},
		{text: "//nolint:tparallel,paralleltest", want: true},
		{text: "//nolint:unused,tparallel", want: true},
		{text: "//nolint:tparallel // reason", want: true},
		{text: "//nolint tparallel", want: true},
		{text: "//nolint tparallel,paralleltest", want: true},
		{text: "//nolint:unused", want: false},
		{text: "//nolint:unused // reason", want: false},
		{text: "// nolint", want: false},
		{text: "// TestX checks x.", want: false},
		{text: "//go:build integration", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			t.Parallel()

			if got := isNolintDirectiveForTparagen(tt.text); got != tt.want {
				t.Errorf("isNolintDirectiveForTparagen(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

// A nolint directive at the end of a line of code applies to the statement
// starting on that line, as it does for the linters: the t.Run call, the for
// loop or the if block holding it, or the whole test function.
func TestGenerateTParallelNolintInline(t *testing.T) {
	t.Parallel()

	runGolden(t, "nolint-inline")
}
