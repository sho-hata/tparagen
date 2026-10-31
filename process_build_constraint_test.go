package tparagen

import "testing"

// A //go:build line with a Go version sets the language version of the file,
// overriding go.mod. Before Go 1.22 the loop variable is shared, so a parallel
// subtest that captures it needs a copy even when go.mod says 1.22 or later.
func TestGenerateTParallelBuildConstraintGoVersion(t *testing.T) {
	t.Parallel()

	runGolden(t, "build-constraint-go-version")
}
