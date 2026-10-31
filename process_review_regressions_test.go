package tparagen

import "testing"

// Subtests whose meaning would change when they run after their parent
// returns stay sequential.
func TestGenerateTParallelKeepsMeaning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase string
		src      string
		want     string
	}{
		{
			testCase: "three-clause loop whose body changes the loop variable",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	for i := 0; i < 4; i++ {
		t.Run("sub", func(t *testing.T) {
			fmt.Println(i)
		})
		i++
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	for i := 0; i < 4; i++ {
		t.Run("sub", func(t *testing.T) {
			fmt.Println(i)
		})
		i++
	}
}
`,
		},
		{
			testCase: "subtest started by a helper closure",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	want := 1
	check := func(name string) {
		t.Run(name, func(t *testing.T) {
			fmt.Println(want)
		})
	}
	check("one")
	want = 2
	check("two")
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	want := 1
	check := func(name string) {
		t.Run(name, func(t *testing.T) {
			fmt.Println(want)
		})
	}
	check("one")
	want = 2
	check("two")
}
`,
		},
		{
			testCase: "parent changes global state and restores it with defer",
			src: `package t

import "testing"

var cfg = "prod"

func TestX(t *testing.T) {
	old := cfg
	cfg = "test"
	defer func() { cfg = old }()
	for _, n := range []string{"a", "b"} {
		t.Run(n, func(t *testing.T) {
			fmt.Println(cfg)
		})
	}
}
`,
			want: `package t

import "testing"

var cfg = "prod"

func TestX(t *testing.T) {
	old := cfg
	cfg = "test"
	defer func() { cfg = old }()
	for _, n := range []string{"a", "b"} {
		t.Run(n, func(t *testing.T) {
			fmt.Println(cfg)
		})
	}
}
`,
		},
		{
			testCase: "variable reassigned between two subtests",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	want := 1
	t.Run("one", func(t *testing.T) {
		fmt.Println(want)
	})
	want = 2
	t.Run("two", func(t *testing.T) {
		fmt.Println(want)
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	want := 1
	t.Run("one", func(t *testing.T) {
		fmt.Println(want)
	})
	want = 2
	t.Run("two", func(t *testing.T) {
		fmt.Println(want)
	})
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
