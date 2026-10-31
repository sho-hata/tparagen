package tparagen

import "testing"

// Before Go 1.22 the loop variable is shared between iterations, so a parallel
// subtest that captures it must get its own copy (`tc := tc`). These cases run
// with needFixLoopVar=true and pin down which copies are inserted.
func TestGenerateTParallelLoopVarCopy(t *testing.T) {
	t.Parallel()

	runGolden(t, "loop-var-copy")
}
