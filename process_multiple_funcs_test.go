package tparagen

import "testing"

// Regression test: the "already has Parallel()/Setenv()" state must not leak
// between test functions of the same file. Before the fix, once a function
// with t.Parallel() or t.Setenv() was seen, no later function in the file got
// t.Parallel() inserted at its top level.
func TestGenerateTParallelMultipleFunctions(t *testing.T) {
	t.Parallel()

	src := `package t

import "testing"

func TestAlreadyParallel(t *testing.T) {
	t.Parallel()
}

func TestAfterParallel(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})
}

func TestHasSetenv(t *testing.T) {
	t.Setenv("X", "y")
}

func TestAfterSetenv(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})
}

func TestAfterSetenvWithRange(t *testing.T) {
	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fmt.Println(tc.name)
		})
	}
}
`

	want := `package t

import "testing"

func TestAlreadyParallel(t *testing.T) {
	t.Parallel()
}

func TestAfterParallel(t *testing.T) {
	t.Parallel()
	t.Run("1", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})
}

func TestHasSetenv(t *testing.T) {
	t.Setenv("X", "y")
}

func TestAfterSetenv(t *testing.T) {
	t.Parallel()
	t.Run("1", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})
}

func TestAfterSetenvWithRange(t *testing.T) {
	t.Parallel()
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
`

	got, err := GenerateTParallel("./testdata/t/t_test.go", []byte(src), false)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != want {
		t.Errorf("result:\n%s\nwant:\n%s", got, want)
	}
}
