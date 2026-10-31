package tparagen

import "testing"

// A test that changes process-wide state (the environment, the working
// directory, the default logger, package-level variables) races with every
// other parallel test, so neither it nor its ancestors get Parallel().
func TestGenerateTParallelGlobalState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase string
		src      string
		want     string
	}{
		{
			testCase: "os.Setenv",
			src: `package t

import (
	"os"
	"testing"
)

func TestX(t *testing.T) {
	os.Setenv("X", "y")
}
`,
			want: `package t

import (
	"os"
	"testing"
)

func TestX(t *testing.T) {
	os.Setenv("X", "y")
}
`,
		},
		{
			testCase: "os.Chdir through an import alias",
			src: `package t

import (
	goos "os"
	"testing"
)

func TestX(t *testing.T) {
	goos.Chdir("/")
}
`,
			want: `package t

import (
	goos "os"
	"testing"
)

func TestX(t *testing.T) {
	goos.Chdir("/")
}
`,
		},
		{
			testCase: "slog.SetDefault",
			src: `package t

import (
	"log/slog"
	"testing"
)

func TestX(t *testing.T) {
	slog.SetDefault(logger)
}
`,
			want: `package t

import (
	"log/slog"
	"testing"
)

func TestX(t *testing.T) {
	slog.SetDefault(logger)
}
`,
		},
		{
			testCase: "package variable assigned in a subtest: only the sibling gets Parallel",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	t.Run("stub", func(t *testing.T) {
		now = func() time.Time { return fixed }
	})
	t.Run("plain", func(t *testing.T) {
		fmt.Println("1")
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Run("stub", func(t *testing.T) {
		now = func() time.Time { return fixed }
	})
	t.Run("plain", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})
}
`,
		},
		{
			testCase: "variable of another package, field and element of a package variable, increment",
			src: `package t

import "testing"

func TestA(t *testing.T) {
	http.DefaultClient = client
}

func TestB(t *testing.T) {
	config.Debug = true
}

func TestC(t *testing.T) {
	registry["x"] = handler
}

func TestD(t *testing.T) {
	counter++
}
`,
			want: `package t

import "testing"

func TestA(t *testing.T) {
	http.DefaultClient = client
}

func TestB(t *testing.T) {
	config.Debug = true
}

func TestC(t *testing.T) {
	registry["x"] = handler
}

func TestD(t *testing.T) {
	counter++
}
`,
		},
		{
			testCase: "local variables are not global state",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	x := 1
	x = 2
	var m map[string]int
	m["a"] = x
	for i, tc := range tests {
		tc.v = i
	}
	f := func(n int) (r int) {
		n++
		r = n
		return
	}
	_ = f
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	x := 1
	x = 2
	var m map[string]int
	m["a"] = x
	for i, tc := range tests {
		tc.v = i
	}
	f := func(n int) (r int) {
		n++
		r = n
		return
	}
	_ = f
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
