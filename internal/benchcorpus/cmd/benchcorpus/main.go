// Command benchcorpus writes a synthetic test corpus for benchmarking tparagen.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sho-hata/tparagen/internal/benchcorpus"
)

func main() {
	var spec benchcorpus.Spec

	dir := flag.String("dir", "", "output directory (required)")
	flag.IntVar(&spec.Packages, "packages", 200, "number of package directories")
	flag.IntVar(&spec.FilesPerPackage, "files", 15, "test files per package")
	flag.IntVar(&spec.TestsPerFile, "tests", 20, "test functions per file")
	flag.IntVar(&spec.NestDepth, "depth", 3, "nesting depth of the nested subtest shape")
	flag.StringVar(&spec.GoVersion, "go-version", "1.21", "go directive written to go.mod")
	flag.Parse()

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "benchcorpus: -dir is required")
		os.Exit(2)
	}

	if err := benchcorpus.Write(*dir, spec); err != nil {
		fmt.Fprintln(os.Stderr, "benchcorpus:", err)
		os.Exit(1)
	}
}
