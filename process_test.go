package tparagen

import "testing"

func TestProcess(t *testing.T) {
	t.Parallel()

	runGolden(t, "process")
}
