package t

import (
	"fmt"
	"testing"
)

func NoTestFunction() {}

func TestingFunctionLooksLikeATestButIsWithParam(i int) {}

func AbcFunctionSuccessful(t *testing.T) {}

func TestFunctionMissingParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}

func TestFunctionHasParallelInMain(t *testing.T) {
	t.Parallel()
	t.Run("hoge", nil)
}

func TestFunctionHasParallelInMainOneTestRunMissingParallel(t *testing.T) {
	t.Parallel()

	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})
}

func TestFunctionMissingParallelAllTests(t *testing.T) {
	t.Run("1", func(x *testing.T) {
		fmt.Println("1")
	})
	t.Run("2", func(t *testing.T) {
		fmt.Println("2")
	})
}

func TestFunctionSuccessAllTests(t *testing.T) {
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

func TestFunctionMissingParallelInMainSubTestHasParallel(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})
}

func TestFunctionMissingParallelInTwoTestMainTestHasParallel(t *testing.T) {
	t.Parallel()

	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})

	t.Run("2", func(t *testing.T) {
		fmt.Println("2")
	})
}

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

func TestMainFunctionMissingParallelHasSetenv(t *testing.T) {
	t.Setenv("TEST", "test")
	t.Run("hoge", nil)
}

func TestSubFunctionMissingParallelHasSetenv(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		t.Setenv("TEST", "test")
		fmt.Println("1")
	})
}

func TestMainAndSubFunctionHasSetenv(t *testing.T) {
	t.Setenv("TEST", "test")
	t.Run("1", func(t *testing.T) {
		t.Setenv("TEST", "test")
		fmt.Println("1")
	})
}

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
