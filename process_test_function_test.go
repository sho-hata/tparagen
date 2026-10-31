package tparagen

import "testing"

// Only functions that `go test` runs as tests are rewritten: methods are run by
// suite runners that may not support parallel tests, and a name such as
// Testable is not a test name.
func TestGenerateTParallelOnlyTestFunctions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase string
		src      string
		want     string
	}{
		{
			testCase: "a method run by a suite runner is left alone",
			src: `package t

import "testing"

type s struct{}

func (s) TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})
}

func Test(t *testing.T) {
	runSuite(t, s{})
}
`,
			want: `package t

import "testing"

type s struct{}

func (s) TestX(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})
}

func Test(t *testing.T) {
	t.Parallel()
	runSuite(t, s{})
}
`,
		},
		{
			testCase: "Test followed by a lower-case letter is not a test",
			src: `package t

import "testing"

func Testable(t *testing.T) {
	fmt.Println("x")
}

func Testé(t *testing.T) {
	fmt.Println("x")
}
`,
			want: `package t

import "testing"

func Testable(t *testing.T) {
	fmt.Println("x")
}

func Testé(t *testing.T) {
	fmt.Println("x")
}
`,
		},
		{
			testCase: "Test followed by an underscore, a digit or an upper-case letter is a test",
			src: `package t

import "testing"

func Test_x(t *testing.T) {
}

func Test1(t *testing.T) {
}

func TestÉ(t *testing.T) {
}
`,
			want: `package t

import "testing"

func Test_x(t *testing.T) {
	t.Parallel()
}

func Test1(t *testing.T) {
	t.Parallel()
}

func TestÉ(t *testing.T) {
	t.Parallel()
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
