package benchcorpus_test

import (
	"bytes"
	"go/format"
	"testing"

	"github.com/sho-hata/tparagen/internal/benchcorpus"
)

func TestFileIsDeterministicAndGofmtClean(t *testing.T) {
	t.Parallel()

	spec := benchcorpus.Spec{TestsPerFile: 21, NestDepth: 30}

	src := benchcorpus.File(spec, 1, 2)
	if !bytes.Equal(src, benchcorpus.File(spec, 1, 2)) {
		t.Error("same arguments produced different sources")
	}

	if bytes.Equal(src, benchcorpus.File(spec, 1, 3)) {
		t.Error("different files produced the same source")
	}

	formatted, err := format.Source(src)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(src, formatted) {
		t.Error("source is not gofmt-clean")
	}
}
