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

When the target code is written for a Go version earlier than 1.22, where the loop variable is shared between iterations (see: https://go.dev/blog/loopvar-preview), code to copy the loop variable is inserted as well. The version is taken from the `go` directive of the `go.mod` nearest to each file, so every module of a multi-module repository uses its own version; use `--min-go-version` to override it.

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
- [x] Subtests started anywhere in a test: in range or three-clause for loops, in if/switch blocks, and inside other subtests.
- [x] Copy loop variables (`tc := tc`) before parallel subtests when the target Go version is earlier than 1.22 (detected from the `go.mod` of each module, or `--min-go-version`)
- [x] Do not insert if `t.Setenv()`, `t.Chdir()` (Go 1.24+) or `cryptotest.SetGlobalRandom()` (Go 1.26+) is called in the test function or in any of its subtests. Those calls panic when the test or one of its ancestors is parallel.
- [x] Do not insert if the test function or any of its subtests changes global state: calls `os.Setenv`/`os.Unsetenv`/`os.Clearenv`/`os.Chdir`, `log.SetOutput`/`log.SetFlags`/`log.SetPrefix` or `slog.SetDefault`/`slog.SetLogLoggerLevel`, or assigns to a package-level variable (of any package), or a field or element of one.
- [x] Keep a subtest sequential if it uses a variable that the function starting it hands to a `defer` statement, such as `srv` in `defer srv.Close()` or `ctx` in `ctx, cancel := context.WithCancel(ctx); defer cancel()`. Parallel subtests only start after that function has returned and its deferred calls have run. Use `t.Cleanup` instead to let them run in parallel.
- [x] Ignore specified directories with cli option -i/-ignore
- [x] nolint comment support: parallel,paralleltest
- [x] Only functions that `go test` runs as tests are rewritten. Methods such as `func (s) TestX(t *testing.T)` are left alone: they are run by suite runners that may set up and check process-wide state around every test. So are functions such as `Testable`, which `go test` does not treat as tests.

### The following cases are not supported
- Don't insert if the test function calls another function that calls `Setenv()`.
- Don't insert if the test function calls another function that changes global state.

### Caveat: global state
tparagen only reads the test code. It cannot see global state that is changed or shared somewhere else: by a helper function, by the code under test, or through a shared database, file or port. Making such tests parallel makes them race with each other.

After running tparagen, run the tests with the race detector several times, and exclude the tests that fail with a nolint comment:

```
$ go test -race -count=5 ./...
```

```go
//nolint:paralleltest // shares the global config with TestY
func TestX(t *testing.T) {
```

## Synopsis
```
$ tparagen
Updated 9 of 10 test files: 112 t.Parallel() added
8 tests left sequential: t.Parallel() is not safe there (t.Setenv, global state, ...). Run with -v to see the files.
✨ Done in 0.02s
```

With `-v`, a line is printed for every file that was changed or has tests left sequential:
```
$ tparagen -v
app_test.go: 23 t.Parallel() added
args_test.go: 6 t.Parallel() added, 2 tests left sequential
...
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
  -v, --[no-]verbose   print a line for every file that was changed or has tests
                       left sequential.
      --min-go-version=MIN-GO-VERSION
                       minimum go version of the target code. ex: 1.21 (default:
                       the go directive of the go.mod nearest to each file.
                       Loop variables are copied before parallel subtests when
                       it is older than 1.22.)

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
