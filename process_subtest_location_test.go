package tparagen

import "testing"

// Subtests get Parallel() wherever t.Run is called: in an if block, in a
// three-clause for loop, or inside another subtest.
func TestGenerateTParallelSubtestLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase       string
		needFixLoopVar bool
		src            string
		want           string
	}{
		{
			testCase: "subtest in an if block",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	if !testing.Short() {
		t.Run("1", func(t *testing.T) {
			fmt.Println("1")
		})
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	if !testing.Short() {
		t.Run("1", func(t *testing.T) {
			t.Parallel()
			fmt.Println("1")
		})
	}
}
`,
		},
		{
			testCase: "subtest in a three-clause for loop",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	cases := []string{"a"}
	for i := 0; i < len(cases); i++ {
		t.Run(cases[i], func(t *testing.T) {
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
	for i := 0; i < len(cases); i++ {
		t.Run(cases[i], func(t *testing.T) {
			t.Parallel()
			fmt.Println(cases[i])
		})
	}
}
`,
		},
		{
			testCase:       "subtest in a three-clause for loop before Go 1.22 copies the loop variable",
			needFixLoopVar: true,
			src: `package t

import "testing"

func TestX(t *testing.T) {
	cases := []string{"a"}
	for i := 0; i < len(cases); i++ {
		t.Run(cases[i], func(t *testing.T) {
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
	for i := 0; i < len(cases); i++ {
		i := i
		t.Run(cases[i], func(t *testing.T) {
			t.Parallel()
			fmt.Println(cases[i])
		})
	}
}
`,
		},
		{
			testCase:       "range subtest inside an if block before Go 1.22 copies the loop variable",
			needFixLoopVar: true,
			src: `package t

import "testing"

func TestX(t *testing.T) {
	if !testing.Short() {
		for _, tc := range []string{"a"} {
			t.Run(tc, func(t *testing.T) {
				fmt.Println(tc)
			})
		}
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	if !testing.Short() {
		for _, tc := range []string{"a"} {
			tc := tc
			t.Run(tc, func(t *testing.T) {
				t.Parallel()
				fmt.Println(tc)
			})
		}
	}
}
`,
		},
		{
			testCase: "nested subtest already parallel: the outer subtest still gets Parallel",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	t.Run("outer", func(t *testing.T) {
		t.Run("inner", func(t *testing.T) {
			t.Parallel()
		})
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	t.Run("outer", func(t *testing.T) {
		t.Parallel()
		t.Run("inner", func(t *testing.T) {
			t.Parallel()
		})
	})
}
`,
		},
		{
			testCase: "nested subtests with their own receiver names",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	t.Run("outer", func(st *testing.T) {
		for _, tc := range []string{"a"} {
			st.Run(tc, func(sst *testing.T) {
				fmt.Println(tc)
			})
		}
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	t.Run("outer", func(st *testing.T) {
		st.Parallel()
		for _, tc := range []string{"a"} {
			st.Run(tc, func(sst *testing.T) {
				sst.Parallel()
				fmt.Println(tc)
			})
		}
	})
}
`,
		},
		{
			testCase: "Setenv in a nested subtest keeps every ancestor serial",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	t.Run("outer", func(t *testing.T) {
		t.Run("inner", func(t *testing.T) {
			t.Setenv("K", "V")
		})
		t.Run("sibling", func(t *testing.T) {
			fmt.Println("sibling")
		})
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Run("outer", func(t *testing.T) {
		t.Run("inner", func(t *testing.T) {
			t.Setenv("K", "V")
		})
		t.Run("sibling", func(t *testing.T) {
			t.Parallel()
			fmt.Println("sibling")
		})
	})
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testCase, func(t *testing.T) {
			t.Parallel()

			got, err := GenerateTParallel("./testdata/t/t_test.go", []byte(tt.src), tt.needFixLoopVar)
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != tt.want {
				t.Errorf("result:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
