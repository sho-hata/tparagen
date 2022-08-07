package t

import (
	"fmt"
	"testing"
)

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
