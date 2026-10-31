package tparagen

import "testing"

// Every range loop of a test gets its subtests handled, not only the last one.
func TestGenerateTParallelMultipleRangeLoops(t *testing.T) {
	t.Parallel()

	runGolden(t, "multiple-range-loops")
}
