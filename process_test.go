package tparagen

import (
	"reflect"
	"testing"
)

func TestProcess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase string
		src      string
		want     string
	}{
		{
			testCase: "no a test function",
			src: `package t

func NoATestFunction() {}
`,
			want: `package t

func NoATestFunction() {}
`,
		},
		{
			testCase: "looks like a test but is with param",
			src: `package t

func TestingFunctionLooksLikeATestButIsWithParam(i int) {}
`,
			want: `package t

func TestingFunctionLooksLikeATestButIsWithParam(i int) {}
`,
		},
		{
			testCase: "test function but empty body",
			src: `package t

func AbcFunctionSuccessful(t *testing.T) {}
`,
			want: `package t

func AbcFunctionSuccessful(t *testing.T) {}
`,
		},
		{
			testCase: "missing called t.Parallel in main test",
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
		},
		{
			testCase: "called t.Parallel in main test",
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
		},
		{
			testCase: "missing called t.Parallel in a sub test",
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
		},
		{
			testCase: "missing called t.Parallel in multiple sub tests",
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
		},
		{
			testCase: "first one test run missing to parallel",
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
		},
		{
			testCase: "second one test run missing call to parallel",
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
		},
		{
			testCase: "missing t.Parallel in range subtest",
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fmt.Println(tc.name)
		})
	}
}
`,
		},
		{
			testCase: "missing t.Parallel in range subtest with does not have test function in range statement",
			src: `package t

import "testing"

func TestFunctionRangeMissingCallToParallel(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
	}{{name: "foo"}}

	// this range loop should be okay as it does not have test Run
	for _, tc := range testCases {
		fmt.Println(tc.name)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fmt.Println(tc.name)
		})
	}
}
`,
			want: `package t

import "testing"

func TestFunctionRangeMissingCallToParallel(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
	}{{name: "foo"}}

	// this range loop should be okay as it does not have test Run
	for _, tc := range testCases {
		fmt.Println(tc.name)
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fmt.Println(tc.name)
		})
	}
}
`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testCase, func(t *testing.T) {
			t.Parallel()
			got, err := Process("./testdata/t/t_test.go", []byte(tt.src))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, []byte(tt.want)) {
				t.Errorf("Process() = \n%v, want\n%v", string(got), tt.want)
			}
		})
	}
}
