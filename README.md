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

In Go versions earlier than 1.21, code to reassign the loop variable is inserted.(see: https://go.dev/blog/loopvar-preview)

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

## Demo
![demo](/doc/tparagen.gif)


### The following cases are supported
- [x] Insert RunParallel helper function into the main/sub test function.
- [x] Loop variables are not re-initialised if the minimum version of Go is less than 1.22
- [x] Do not insert if `t.Setenv()` is called in the test function
- [x] Ignore specified directories with cli option -i/-ignore
- [x] nolint comment support: parallel,paralleltest

### The following cases are not supported
- Don't insert if the test function calls another function that calls `Setenv()`.

## Synopsis
```
$ tparagen
```

## Options
```
$ tparagen --help
usage: tparagen [<flags>]


Flags:
  --[no-]help            Show context-sensitive help (also try --help-long and --help-man).
  --ignore=IGNORE        ignore directory names. ex: foo,bar,baz (testdata directory is always ignored.)
  --min-go-version=1.21  minimum go version

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
