package tparagen

import "testing"

// t.Setenv, t.Chdir (Go 1.24+) and cryptotest.SetGlobalRandom (Go 1.26+)
// panic when the calling test, or any of its ancestors, has called
// t.Parallel(). These cases pin down that tparagen never produces such a
// combination, while still parallelizing what is allowed (e.g. subtests of a
// parent that calls t.Setenv).
func TestGenerateTParallelParallelDenyingCalls(t *testing.T) {
	t.Parallel()

	runGolden(t, "parallel-denying-calls")
}
