package tparagen

import "testing"

// A Parallel() call that is not a top-level statement of the test still makes
// the test parallel. Inserting another one panics at run time with
// "testing: t.Parallel called multiple times".
func TestGenerateTParallelExistingParallelInBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase string
		src      string
		want     string
	}{
		{
			testCase: "Parallel inside an if block",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	if !testing.Short() {
		t.Parallel()
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	if !testing.Short() {
		t.Parallel()
	}
}
`,
		},
		{
			testCase: "Parallel inside an else block",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	if testing.Short() {
		fmt.Println("short")
	} else {
		t.Parallel()
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	if testing.Short() {
		fmt.Println("short")
	} else {
		t.Parallel()
	}
}
`,
		},
		{
			testCase: "Parallel inside a switch case",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	switch os.Getenv("MODE") {
	case "serial":
	default:
		t.Parallel()
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	switch os.Getenv("MODE") {
	case "serial":
	default:
		t.Parallel()
	}
}
`,
		},
		{
			testCase: "Parallel inside a subtest does not count for the parent",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	if true {
		t.Run("1", func(t *testing.T) {
			t.Parallel()
		})
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	if true {
		t.Run("1", func(t *testing.T) {
			t.Parallel()
		})
	}
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
