package tparagen

import "testing"

// t.Setenv, t.Chdir (Go 1.24+) and cryptotest.SetGlobalRandom (Go 1.26+)
// panic when the calling test, or any of its ancestors, has called
// t.Parallel(). These cases pin down that tparagen never produces such a
// combination, while still parallelizing what is allowed (e.g. subtests of a
// parent that calls t.Setenv).
func TestGenerateTParallelParallelDenyingCalls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase string
		src      string
		want     string
	}{
		{
			testCase: "Setenv in a subtest: neither the parent nor that subtest, but the sibling, gets Parallel",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	t.Run("env", func(t *testing.T) {
		t.Setenv("X", "y")
	})
	t.Run("plain", func(t *testing.T) {
		fmt.Println("1")
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Run("env", func(t *testing.T) {
		t.Setenv("X", "y")
	})
	t.Run("plain", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})
}
`,
		},
		{
			testCase: "Setenv in a nested subtest forbids Parallel on every ancestor",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	t.Run("outer", func(t *testing.T) {
		t.Run("inner", func(t *testing.T) {
			t.Setenv("X", "y")
		})
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Run("outer", func(t *testing.T) {
		t.Run("inner", func(t *testing.T) {
			t.Setenv("X", "y")
		})
	})
}
`,
		},
		{
			testCase: "Setenv inside a nested block of the main test",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Setenv("X", "y")
	}
	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Setenv("X", "y")
	}
	t.Run("1", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})
}
`,
		},
		{
			testCase: "Chdir in the main test: subtests may still be parallel",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	t.Chdir("testdata")
	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Chdir("testdata")
	t.Run("1", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})
}
`,
		},
		{
			testCase: "Chdir in a subtest",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		t.Chdir("testdata")
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		t.Chdir("testdata")
	})
}
`,
		},
		{
			testCase: "cryptotest.SetGlobalRandom in the main test",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	cryptotest.SetGlobalRandom(t, 1)
	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	cryptotest.SetGlobalRandom(t, 1)
	t.Run("1", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})
}
`,
		},
		{
			testCase: "Setenv in a range subtest with a receiver named x",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		t.Run(tc.name, func(x *testing.T) {
			x.Setenv("X", "y")
			fmt.Println(tc.name)
		})
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		t.Run(tc.name, func(x *testing.T) {
			x.Setenv("X", "y")
			fmt.Println(tc.name)
		})
	}
}
`,
		},
		{
			testCase: "Chdir in the main test with range subtests",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	t.Chdir("testdata")
	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fmt.Println(tc.name)
		})
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Chdir("testdata")
	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fmt.Println(tc.name)
		})
	}
}
`,
		},
		{
			testCase: "os.Setenv is not a *testing.T call and does not block Parallel",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	os.Setenv("X", "y")
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	os.Setenv("X", "y")
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
