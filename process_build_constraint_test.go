package tparagen

import "testing"

// A //go:build line with a Go version sets the language version of the file,
// overriding go.mod. Before Go 1.22 the loop variable is shared, so a parallel
// subtest that captures it needs a copy even when go.mod says 1.22 or later.
func TestGenerateTParallelBuildConstraintGoVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase string
		src      string
		want     string
	}{
		{
			testCase: "go1.8 build constraint copies the loop variable",
			src: `//go:build go1.8

package t

import "testing"

func TestX(t *testing.T) {
	for _, tc := range []string{"a"} {
		t.Run(tc, func(t *testing.T) {
			fmt.Println(tc)
		})
	}
}
`,
			want: `//go:build go1.8

package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	for _, tc := range []string{"a"} {
		tc := tc
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			fmt.Println(tc)
		})
	}
}
`,
		},
		{
			testCase: "go1.22 build constraint does not",
			src: `//go:build go1.22 && linux

package t

import "testing"

func TestX(t *testing.T) {
	for _, tc := range []string{"a"} {
		t.Run(tc, func(t *testing.T) {
			fmt.Println(tc)
		})
	}
}
`,
			want: `//go:build go1.22 && linux

package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	for _, tc := range []string{"a"} {
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			fmt.Println(tc)
		})
	}
}
`,
		},
		{
			testCase: "build constraint without a Go version does not",
			src: `//go:build linux

package t

import "testing"

func TestX(t *testing.T) {
	for _, tc := range []string{"a"} {
		t.Run(tc, func(t *testing.T) {
			fmt.Println(tc)
		})
	}
}
`,
			want: `//go:build linux

package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	for _, tc := range []string{"a"} {
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			fmt.Println(tc)
		})
	}
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testCase, func(t *testing.T) {
			t.Parallel()

			// needFixLoopVar=false: go.mod says 1.22 or later.
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
