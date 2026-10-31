package tparagen

import "testing"

// Subtests whose meaning would change when they run after their parent
// returns stay sequential.
func TestGenerateTParallelKeepsMeaning(t *testing.T) {
	t.Parallel()

	runGolden(t, "keeps-meaning")
}
