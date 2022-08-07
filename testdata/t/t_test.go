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
