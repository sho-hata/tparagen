package tparagen

import "testing"

// A parallel subtest only starts after the function that started it has
// returned, so after that function's deferred calls have run. Subtests that use
// what a defer statement closes, cancels or waits for stay sequential.
func TestGenerateTParallelDeferredValues(t *testing.T) {
	t.Parallel()

	runGolden(t, "deferred-values")
}
