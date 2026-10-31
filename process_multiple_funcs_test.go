package tparagen

import "testing"

// Regression test: the "already has Parallel()/Setenv()" state must not leak
// between test functions of the same file. Before the fix, once a function
// with t.Parallel() or t.Setenv() was seen, no later function in the file got
// t.Parallel() inserted at its top level.
func TestGenerateTParallelMultipleFunctions(t *testing.T) {
	t.Parallel()

	runGolden(t, "multiple-functions")
}
