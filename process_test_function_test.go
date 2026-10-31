package tparagen

import "testing"

// Only functions that `go test` runs as tests are rewritten: methods are run by
// suite runners that may not support parallel tests, and a name such as
// Testable is not a test name.
func TestGenerateTParallelOnlyTestFunctions(t *testing.T) {
	t.Parallel()

	runGolden(t, "only-test-functions")
}
