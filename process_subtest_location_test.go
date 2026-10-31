package tparagen

import "testing"

// Subtests get Parallel() wherever t.Run is called: in an if block, in a
// three-clause for loop, or inside another subtest.
func TestGenerateTParallelSubtestLocation(t *testing.T) {
	t.Parallel()

	runGolden(t, "subtest-location")
}
