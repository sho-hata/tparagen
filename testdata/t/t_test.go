package t

import (
	"testing"
)

func TestFunctionDoNotCalledParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}

func TestFunctionCalledParallelInMain(t *testing.T) {
	t.Parallel()
	t.Run("hoge", nil)
}
