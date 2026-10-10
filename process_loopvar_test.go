package tparagen

import "testing"

// Before Go 1.22 the loop variable is shared between iterations, so a parallel
// subtest that captures it must get its own copy (`tc := tc`). These cases run
// with needFixLoopVar=true and pin down which copies are inserted.
func TestGenerateTParallelLoopVarCopy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase string
		src      string
		want     string
	}{
		{
			testCase: "key and value are both captured",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	cases := []string{"a"}
	for i, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			fmt.Println(i, tc)
		})
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	cases := []string{"a"}
	for i, tc := range cases {
		i := i
		tc := tc
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			fmt.Println(i, tc)
		})
	}
}
`,
		},
		{
			testCase: "only the key is captured",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	cases := []string{"a"}
	for i := range cases {
		t.Run("x", func(t *testing.T) {
			fmt.Println(cases[i])
		})
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	cases := []string{"a"}
	for i := range cases {
		i := i
		t.Run("x", func(t *testing.T) {
			t.Parallel()
			fmt.Println(cases[i])
		})
	}
}
`,
		},
		{
			testCase: "a value that is already copied is not copied again",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	cases := []string{"a"}
	for _, tc := range cases {
		tc := tc
		t.Run(tc, func(t *testing.T) {
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
			testCase: "a multi-assignment copy is recognised",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	cases := []string{"a"}
	for i, tc := range cases {
		i, tc := i, tc
		t.Run(tc, func(t *testing.T) {
			fmt.Println(i, tc)
		})
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	cases := []string{"a"}
	for i, tc := range cases {
		i, tc := i, tc
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			fmt.Println(i, tc)
		})
	}
}
`,
		},
		{
			testCase: "an unrelated short variable declaration does not count as a copy",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	cases := []string{"a"}
	y := 1
	for _, tc := range cases {
		x := y
		t.Run(tc, func(t *testing.T) {
			fmt.Println(tc, x)
		})
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	cases := []string{"a"}
	y := 1
	for _, tc := range cases {
		tc := tc
		x := y
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			fmt.Println(tc, x)
		})
	}
}
`,
		},
		{
			testCase: "a loop variable used only as the subtest name is not copied",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	cases := []string{"a"}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			fmt.Println("hi")
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
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			fmt.Println("hi")
		})
	}
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testCase, func(t *testing.T) {
			t.Parallel()

			got, err := GenerateTParallel("./testdata/t/t_test.go", []byte(tt.src), true)
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != tt.want {
				t.Errorf("result:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
