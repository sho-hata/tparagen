package tparagen

import "testing"

// t.Setenv and t.Chdir reached through a function value, a table entry or a
// testing value stored in a struct forbid Parallel() like a direct call does.
func TestGenerateTParallelFunctionValues(t *testing.T) {
	t.Parallel()

	runGolden(t, "function-values")
}
