package tparagen

import (
	"reflect"
	"testing"
)

func TestProcess(t *testing.T) {
	t.Parallel()
	// t.Parallel()

	tests := []struct {
		testCase string
		src      string
		want     string
	}{
		{
			testCase: "no a test function",
			src: `package t

func NoTestFunction() {}
`,
			want: `package t

func NoTestFunction() {}
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

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
			want: `package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Parallel()
	t.Run("hoge", nil)
}
`,
		},
		{
			testCase: "called t.Parallel in main test",
			src: `package t

import "testing"

func TestFunctionHasParallelInMain(t *testing.T) {
	t.Parallel()
	t.Run("hoge", nil)
}`,
			want: `package t

import "testing"

func TestFunctionHasParallelInMain(t *testing.T) {
	t.Parallel()
	t.Run("hoge", nil)
}
`,
		},
		{
			testCase: "missing called t.Parallel in a sub test",
			src: `package t

import "testing"

func TestFunctionHasParallelInMainOneTestRunMissingHasParallel(t *testing.T) {
	t.Parallel()

	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})
}
`,
			want: `package t

import "testing"

func TestFunctionHasParallelInMainOneTestRunMissingHasParallel(t *testing.T) {
	t.Parallel()

	t.Run("1", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})
}
`,
		},
		{
			testCase: "missing called t.Parallel in all tests not range",
			src: `package t

import "testing"

func TestFunctionMissingParallelAllTests(t *testing.T) {
	t.Run("1", func(x *testing.T) {
		fmt.Println("1")
	})
	t.Run("2", func(t *testing.T) {
		fmt.Println("2")
	})
}
`,
			want: `package t

import "testing"

func TestFunctionMissingParallelAllTests(t *testing.T) {
	t.Parallel()
	t.Run("1", func(x *testing.T) {
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
			testCase: "t",
			src: `package t

import "testing"

func TestFunctionMissingParallelInMainSubTestHasParallel(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})
}
`,
			want: `package t

import "testing"

func TestFunctionMissingParallelInMainSubTestHasParallel(t *testing.T) {
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

func TestFunctionMissingParallelInTwoTestMainTestHasParallel(t *testing.T) {
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

func TestFunctionMissingParallelInTwoTestMainTestHasParallel(t *testing.T) {
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

func TestFunctionMissingParallelInFirstOneTestMainTestHasParallel(t *testing.T) {
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

func TestFunctionMissingParallelInFirstOneTestMainTestHasParallel(t *testing.T) {
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

func TestFunctionMissingParallelInSecondOneTestMainTestHasParallel(t *testing.T) {
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

func TestFunctionMissingParallelInSecondOneTestMainTestHasParallel(t *testing.T) {
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
			testCase: "successful range test",
			src: `package t

import "testing"

func TestFunctionSuccessfulRangeTest(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(x *testing.T) {
			x.Parallel()
			fmt.Println(tc.name)
		})
	}
}
`,
			want: `package t

import "testing"

func TestFunctionSuccessfulRangeTest(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(x *testing.T) {
			x.Parallel()
			fmt.Println(tc.name)
		})
	}
}
`,
		},
		{
			testCase: "missing t.Parallel in range subtest",
			src: `package t

import "testing"

func TestFunctionMissingParallelRangeNotUsingRangeValueInTRun(t *testing.T) {
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

func TestFunctionMissingParallelRangeNotUsingRangeValueInTRun(t *testing.T) {
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
		{
			testCase: "missing t.Parallel in main test function has t.Setenv",
			src: `package t

import "testing"

func TestMainFunctionMissingParallelHasSetenv(t *testing.T) {
	t.Setenv("TEST", "test")
	t.Run("hoge", nil)
}
`,
			want: `package t

import "testing"

func TestMainFunctionMissingParallelHasSetenv(t *testing.T) {
	t.Setenv("TEST", "test")
	t.Run("hoge", nil)
}
`,
		},
		{
			testCase: "missing t.Parallel in sub test function has t.Setenv",
			src: `package t

import "testing"

func TestSubFunctionMissingParallelHasSetenv(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		t.Setenv("TEST", "test")
		fmt.Println("1")
	})
}
`,
			want: `package t

import "testing"

func TestSubFunctionMissingParallelHasSetenv(t *testing.T) {
	t.Parallel()
	t.Run("1", func(t *testing.T) {
		t.Setenv("TEST", "test")
		fmt.Println("1")
	})
}
`,
		},
		{
			testCase: "missing t.Parallel in sub test function Main Test has t.Setenv",
			src: `package t

import "testing"

func TestSubFunctionMissingParallelSubTestHasSetenv(t *testing.T) {
	t.Setenv("TEST", "test")
	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})
}
`,
			want: `package t

import "testing"

func TestSubFunctionMissingParallelSubTestHasSetenv(t *testing.T) {
	t.Setenv("TEST", "test")
	t.Run("1", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})
}
`,
		},
		{
			testCase: "main & sub test function has t.Setenv",
			src: `package t

import "testing"

func TestMainAndSubFunctionHasSetenv(t *testing.T) {
	t.Setenv("TEST", "test")
	t.Run("1", func(t *testing.T) {
		t.Setenv("TEST", "test")
		fmt.Println("1")
	})
}
`,
			want: `package t

import "testing"

func TestMainAndSubFunctionHasSetenv(t *testing.T) {
	t.Setenv("TEST", "test")
	t.Run("1", func(t *testing.T) {
		t.Setenv("TEST", "test")
		fmt.Println("1")
	})
}
`,
		},
		{
			testCase: "main test function has t.Setenv with range statement",
			src: `package t

import "testing"

func TestFunctionMainHasSetenvWithRangeTest(t *testing.T) {
	t.Setenv("TEST", "test")

	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(x *testing.T) {
			x.Parallel()
			fmt.Println(tc.name)
		})
	}
}
`,
			want: `package t

import "testing"

func TestFunctionMainHasSetenvWithRangeTest(t *testing.T) {
	t.Setenv("TEST", "test")

	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(x *testing.T) {
			x.Parallel()
			fmt.Println(tc.name)
		})
	}
}
`,
		},
		{
			testCase: "sub test function has t.Setenv with range statement",
			src: `package t

import "testing"

func TestFunctionSubHasSetenvWithRangeTest(t *testing.T) {
	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		t.Run(tc.name, func(x *testing.T) {
			x.Setenv("TEST", "test")
			fmt.Println(tc.name)
		})
	}
}
`,
			want: `package t

import "testing"

func TestFunctionSubHasSetenvWithRangeTest(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		t.Run(tc.name, func(x *testing.T) {
			x.Setenv("TEST", "test")
			fmt.Println(tc.name)
		})
	}
}
`,
		},
		{
			testCase: "main & sub test function has t.Setenv with range statement",
			src: `package t

import "testing"

func TestFunctionMainAndSubHasSetenvWithRangeTest(t *testing.T) {
	t.Setenv("TEST", "test")

	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		t.Run(tc.name, func(x *testing.T) {
			x.Setenv("TEST", "test")
			fmt.Println(tc.name)
		})
	}
}
`,
			want: `package t

import "testing"

func TestFunctionMainAndSubHasSetenvWithRangeTest(t *testing.T) {
	t.Setenv("TEST", "test")

	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		t.Run(tc.name, func(x *testing.T) {
			x.Setenv("TEST", "test")
			fmt.Println(tc.name)
		})
	}
}
`,
		},
		{
			testCase: "ignore all lint to file",
			src: `//nolint
package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
			want: `//nolint
package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
		},
		{
			testCase: "ignore paralleltest lint to file",
			src: `//nolint paralleltest
package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
			want: `//nolint paralleltest
package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
		},
		{
			testCase: "ignore tparallel lint to file",
			src: `//nolint tparallel
package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
			want: `//nolint tparallel
package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
		},
		{
			testCase: "ignore tparallel and paralleltest lint to file",
			src: `//nolint tparallel,paralleltest
package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
			want: `//nolint tparallel,paralleltest
package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
		},
		{
			testCase: "ignore paralleltest lint to file",
			src: `//nolint:paralleltest
package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
			want: `//nolint:paralleltest
package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
		},
		{
			testCase: "ignore tparallel lint to file",
			src: `//nolint:tparallel
package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
			want: `//nolint:tparallel
package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
		},
		{
			testCase: "ignore tparallel and paralleltest lint to file",
			src: `//nolint:tparallel,paralleltest
package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
			want: `//nolint:tparallel,paralleltest
package t

import "testing"

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
		},
		{
			testCase: "ignore tparallel and paralleltest lint to main test",
			src: `package t

import "testing"

//nolint:tparallel,paralleltest
func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
			want: `package t

import "testing"

//nolint:tparallel,paralleltest
func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}
`,
		},
		{
			testCase: "ignore tparallel and paralleltest lint to main test once",
			src: `package t

import "testing"

//nolint:tparallel,paralleltest
func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
			want: `package t

import "testing"

//nolint:tparallel,paralleltest
func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Parallel()
	t.Run("hoge", nil)
}
`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testCase, func(t *testing.T) {
			t.Parallel()

			got, err := Process("./testdata/t/t_test.go", []byte(tt.src), true)
			if err != nil {
				t.Fatal(err.Error())
			}
			if !reflect.DeepEqual(got, []byte(tt.want)) {
				t.Errorf("result:\n%v, want:\n%v", string(got), tt.want)
			}
		})
	}
}
