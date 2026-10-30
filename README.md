# tparagen
tparagen inserts the `T.Parallel()` method from the testing package into a test function in a specific source file or an entire directory.

[![ci](https://github.com/sho-hata/tparagen/actions/workflows/ci.yml/badge.svg)](https://github.com/sho-hata/tparagen/actions/workflows/ci.yml)

## Background
To run go tests in concurrency, you need to the `T.Parallel()` method from the testing package into the main/sub test you want to run in concurrency.

```go
func SampleTest(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// do anything...
		})
	}
}
```

If there is your application in production already, you must add a `T.Parallel()` into any main/sub test. It is a very time-consuming and tedious task.

## Demo
![demo](/doc/tparagen.gif)

## Description
tparagen is cli tool for insert the `T.Parallel()` method from the testing package into all main/sub test in specified directory.

Before code is below,

```go
package sample

import (
	"fmt"
	"testing"
)

func SampleTest(t *testing.T) {

	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fmt.Println(tc.name)
		})
	}
}
```

After execute `tparagen`, modified code is below.

```go
package test

import (
	"fmt"
	"testing"
)

func SampleTest(t *testing.T) {
	t.Parallel() // <- inserted

	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel() // <- inserted
			fmt.Println(tc.name)
		})
	}
}
```

When the target code is written for a Go version earlier than 1.22, where the loop variable is shared between iterations (see: https://go.dev/blog/loopvar-preview), code to copy the loop variable is inserted as well. The version is taken from the `go` directive of the nearest `go.mod`; use `--min-go-version` to override it.

```go
package test

import (
	"fmt"
	"testing"
)

func SampleTest(t *testing.T) {
	t.Parallel() // <- inserted

	testCases := []struct {
		name string
	}{{name: "foo"}}
	for _, tc := range testCases {
		tc := tc // <- inserted
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel() // <- inserted
			fmt.Println(tc.name)
		})
	}
}
```


### The following cases are supported
- [x] Insert RunParallel helper function into the main/sub test function.
- [x] Copy loop variables (`tc := tc`) before parallel subtests when the target Go version is earlier than 1.22 (detected from `go.mod`, or `--min-go-version`)
- [x] Do not insert if `t.Setenv()`, `t.Chdir()` (Go 1.24+) or `cryptotest.SetGlobalRandom()` (Go 1.26+) is called in the test function or in any of its subtests. Those calls panic when the test or one of its ancestors is parallel.
- [x] Ignore specified directories with cli option -i/-ignore
- [x] nolint comment support: parallel,paralleltest

### The following cases are not supported
- Don't insert if the test function calls another function that calls `Setenv()`.

## Performance
On [kubernetes/kubernetes](https://github.com/kubernetes/kubernetes), tparagen rewrites **3,043 of its 3,611 test files in about 1.7 seconds**, using about 180 MiB of memory. The result still type-checks: `go vet` over the whole repository (the main module and all 35 staging modules) reports exactly the same findings before and after the rewrite.

| Repository | Test files rewritten | `t.Parallel()` inserted | Wall time (mean ± σ) | Peak memory (mean) |
|:---|---:|---:|---:|---:|
| kubernetes/kubernetes @ `25b8637d` | 3,043 / 3,611 | 19,318 | 1.71 s ± 0.03 s | 180 MiB |

Measured with [hyperfine](https://github.com/sharkdp/hyperfine) (10 runs after 1 warmup run) on an Apple M4 Pro (12 cores). Every run starts from a clean checkout (`git checkout -- .`), so each run really rewrites all files. Numbers depend on your machine.

tparagen only checks that the rewritten tests compile. Making a test parallel can still break it at run time, for example when it changes global state such as feature gates through a helper function. Run your tests after the rewrite.

To measure a generated corpus of any size (requires hyperfine):

```
$ scripts/benchmark.sh
$ PACKAGES=1000 FILES=10 scripts/benchmark.sh
```

Every pull request runs the Go benchmarks for the pull request and for its base branch on the same machine. The check fails when time, allocations or peak memory get worse by more than 10% with statistical significance (see [`.github/workflows/bench.yml`](.github/workflows/bench.yml)).

## Synopsis
```
$ tparagen
```

## Options
```
$ tparagen --help
usage: tparagen [<flags>]


Flags:
  -h, --[no-]help      Show context-sensitive help (also try --help-long and
                       --help-man).
      --ignore=IGNORE  ignore directory names. ex: foo,bar,baz (testdata
                       directory is always ignored.)
      --min-go-version=MIN-GO-VERSION
                       minimum go version of the target code. ex: 1.21 (default:
                       the go directive of the nearest go.mod. Loop variables
                       are copied before parallel subtests when it is older than
                       1.22.)

```
## Installation
```
go install github.com/sho-hata/tparagen/cmd/tparagen@latest
```


## Contribution
Bug reports and pull requests are welcome on GitHub at https://github.com/sho-hata/tparagen. This project is intended to be a safe, welcoming space for collaboration, and contributors are expected to adhere to the code of conduct.

## License
[MIT](https://github.com/sho-hata/tparagen/blob/main/LICENSE)

## Code of Conduct
Everyone interacting in the tparagen project's codebases, issue trackers, chat rooms and mailing lists is expected to follow the code of conduct.

## Author
[sho-hata](https://github.com/sho-hata)
