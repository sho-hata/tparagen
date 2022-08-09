package tparagen

import (
	"reflect"
	"testing"
)

func TestProcess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase string
		filename string
		src      string
		want     string
		wantErr  bool
	}{
		{
			testCase: "no a test function",
			filename: "/testdata/t/t_test.go",
			src: `package t

func NoATestFunction() {}
`,
			want: `package t

func NoATestFunction() {}
`,
			wantErr: false,
		},
		{
			testCase: "looks like a test but is with param",
			filename: "/testdata/t/t_test.go",
			src: `package t

func TestingFunctionLooksLikeATestButIsWithParam(i int) {}
`,
			want: `package t

func TestingFunctionLooksLikeATestButIsWithParam(i int) {}
`,
			wantErr: false,
		},
		{
			testCase: "test function but empty body",
			filename: "/testdata/t/t_test.go",
			src: `package t

func AbcFunctionSuccessful(t *testing.T) {}
`,
			want: `package t

func AbcFunctionSuccessful(t *testing.T) {}
`,
			wantErr: false,
		},
		{
			testCase: "missing called t.Parallel in main test",
			filename: "./testdata/t/t_test.go",
			src: `package t

import "testing"

func TestFunctionDoNotCalledParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
			want: `package t

import "testing"

func TestFunctionDoNotCalledParallelInMain(t *testing.T) {
	t.Parallel()
	t.Run("hoge", nil)
}
`,
			wantErr: false,
		},
		{
			testCase: "called t.Parallel in main test",
			filename: "./testdata/t/t_test.go",
			src: `package t

import "testing"

func TestFunctionCalledParallelInMain(t *testing.T) {
	t.Parallel()
	t.Run("hoge", nil)
}`,
			want: `package t

import "testing"

func TestFunctionCalledParallelInMain(t *testing.T) {
	t.Parallel()
	t.Run("hoge", nil)
}
`,
			wantErr: false,
		},
		{
			testCase: "missing called t.Parallel in a sub test",
			filename: "./testdata/t/t_test.go",
			src: `package t

import "testing"

func TestFunctionOneTestRunMissingCallToParallel(t *testing.T) {
	t.Parallel()

	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})
}
`,
			want: `package t

import "testing"

func TestFunctionOneTestRunMissingCallToParallel(t *testing.T) {
	t.Parallel()

	t.Run("1", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})
}
`,
			wantErr: false,
		},
		{
			testCase: "missing called t.Parallel in multiple sub tests",
			filename: "./testdata/t/t_test.go",
			src: `package t

import "testing"

func TestFunctionTwoTestRunMissingCallToParallel(t *testing.T) {
	t.Parallel()

	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})

	t.Run("2", func(t *testing.T) {
		fmt.Println("2")
	})
}
`,
			want: `package t

import "testing"

func TestFunctionTwoTestRunMissingCallToParallel(t *testing.T) {
	t.Parallel()

	t.Run("1", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})

	t.Run("2", func(t *testing.T) {
		t.Parallel()
		fmt.Println("2")
	})
}
`,
			wantErr: false,
		},
		{
			testCase: "first one test run missing to parallel",
			filename: "./testdata/t/t_test.go",
			src: `package t

import "testing"

func TestFunctionFirstOneTestRunMissingCallToParallel(t *testing.T) {
	t.Parallel()

	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})

	t.Run("2", func(t *testing.T) {
		t.Parallel()
		fmt.Println("2")
	})
}
`,
			want: `package t

import "testing"

func TestFunctionFirstOneTestRunMissingCallToParallel(t *testing.T) {
	t.Parallel()

	t.Run("1", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})

	t.Run("2", func(t *testing.T) {
		t.Parallel()
		fmt.Println("2")
	})
}
`,
			wantErr: false,
		},
		{
			testCase: "second one test run missing call to parallel",
			filename: "./testdata/t/t_test.go",
			src: `package t

import "testing"

func TestFunctionSecondOneTestRunMissingCallToParallel(t *testing.T) {
	t.Parallel()

	t.Run("1", func(x *testing.T) {
		x.Parallel()
		fmt.Println("1")
	})

	t.Run("2", func(t *testing.T) {
		fmt.Println("2")
	})
}
`,
			want: `package t

import "testing"

func TestFunctionSecondOneTestRunMissingCallToParallel(t *testing.T) {
	t.Parallel()

	t.Run("1", func(x *testing.T) {
		x.Parallel()
		fmt.Println("1")
	})

	t.Run("2", func(t *testing.T) {
		t.Parallel()
		fmt.Println("2")
	})
}
`,
			wantErr: false,
		},
		{
			testCase: "",
			filename: "./testdata/t/t_test.go",
			src: `package t

import "testing"

func TestFunctionMissingCallToParallelAndRangeNotUsingRangeValueInTDotRun(t *testing.T) {
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

func TestFunctionMissingCallToParallelAndRangeNotUsingRangeValueInTDotRun(t *testing.T) {
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
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testCase, func(t *testing.T) {
			t.Parallel()
			got, err := Process(tt.filename, []byte(tt.src))
			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !reflect.DeepEqual(got, []byte(tt.want)) {
				t.Errorf("Process() = \n%v, want\n%v", string(got), tt.want)
				// t.Errorf("Process() = \n%v, want\n%v", got, []byte(tt.want))
			}
		})
	}
}
