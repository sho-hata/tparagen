package tparagen

import "testing"

// A Parallel() call that is not a top-level statement of the test still makes
// the test parallel. Inserting another one panics at run time with
// "testing: t.Parallel called multiple times".
func TestGenerateTParallelExistingParallelInBlock(t *testing.T) {
	t.Parallel()

	runGolden(t, "existing-parallel-in-block")
}
