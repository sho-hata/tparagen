package tparagen

import "testing"

// Tests whose *testing.T parameter is unnamed or the blank identifier have no
// name to call Parallel() on: `_.Parallel()` does not compile, and an unnamed
// parameter used to crash the tool with an index-out-of-range panic.
func TestGenerateTParallelUnusableReceiver(t *testing.T) {
	t.Parallel()

	runGolden(t, "unusable-receiver")
}
