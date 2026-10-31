package tparagen

import "testing"

// A test that changes process-wide state (the environment, the working
// directory, the default logger, package-level variables) races with every
// other parallel test, so neither it nor its ancestors get Parallel().
func TestGenerateTParallelGlobalState(t *testing.T) {
	t.Parallel()

	runGolden(t, "global-state")
}
