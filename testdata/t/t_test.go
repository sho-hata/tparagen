package t

import (
	"fmt"
	"testing"
)

func NoATestFunction() {}

func TestingFunctionLooksLikeATestButIsWithParam(i int) {}

func AbcFunctionSuccessful(t *testing.T) {}

func TestFunctionDoNotCalledParallelInMain(t *testing.T) {
}

func TestFunctionCalledParallelInMain(t *testing.T) {
	t.Parallel()
}

func TestFunctionOneTestRunMissingCallToParallel(t *testing.T) {
	t.Parallel()

	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})
}

func TestFunctionTwoTestRunMissingCallToParallel(t *testing.T) {
	t.Parallel()

	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})

	t.Run("2", func(t *testing.T) {
		fmt.Println("2")
	})
}

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
