package tparagen

import "testing"

// Tests whose *testing.T parameter is unnamed or the blank identifier have no
// name to call Parallel() on: `_.Parallel()` does not compile, and an unnamed
// parameter used to crash the tool with an index-out-of-range panic.
func TestGenerateTParallelUnusableReceiver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase string
		src      string
		want     string
	}{
		{
			testCase: "blank receiver in the main test is left alone",
			src: `package t

import "testing"

func TestX(_ *testing.T) {
	fmt.Println("x")
}
`,
			want: `package t

import "testing"

func TestX(_ *testing.T) {
	fmt.Println("x")
}
`,
		},
		{
			testCase: "blank receiver in a subtest: only the parent gets Parallel",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	t.Run("1", func(_ *testing.T) {
		fmt.Println("1")
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	t.Run("1", func(_ *testing.T) {
		fmt.Println("1")
	})
}
`,
		},
		{
			testCase: "blank receiver in a range subtest: only the parent gets Parallel",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	cases := []string{"a"}
	for _, tc := range cases {
		t.Run(tc, func(_ *testing.T) {
			fmt.Println(tc)
		})
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	cases := []string{"a"}
	for _, tc := range cases {
		t.Run(tc, func(_ *testing.T) {
			fmt.Println(tc)
		})
	}
}
`,
		},
		{
			testCase: "unnamed parameter does not panic and is left alone",
			src: `package t

import "testing"

func TestX(*testing.T) {
	fmt.Println("x")
}
`,
			want: `package t

import "testing"

func TestX(*testing.T) {
	fmt.Println("x")
}
`,
		},
		{
			testCase: "declaration without a body is left alone",
			src: `package t

import "testing"

func TestX(t *testing.T)
`,
			want: `package t

import "testing"

func TestX(t *testing.T)
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
