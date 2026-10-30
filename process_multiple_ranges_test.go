package tparagen

import "testing"

// Every range loop of a test gets its subtests handled, not only the last one.
func TestGenerateTParallelMultipleRangeLoops(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase       string
		needFixLoopVar bool
		src            string
		want           string
	}{
		{
			testCase: "two range loops",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	for _, tc := range []string{"a"} {
		t.Run(tc, func(t *testing.T) {
			fmt.Println(tc)
		})
	}
	for _, tc := range []string{"b"} {
		t.Run(tc, func(t *testing.T) {
			fmt.Println(tc)
		})
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	for _, tc := range []string{"a"} {
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			fmt.Println(tc)
		})
	}
	for _, tc := range []string{"b"} {
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			fmt.Println(tc)
		})
	}
}
`,
		},
		{
			testCase:       "two range loops before Go 1.22 copy the loop variable in each",
			needFixLoopVar: true,
			src: `package t

import "testing"

func TestX(t *testing.T) {
	for _, tc := range []string{"a"} {
		t.Run(tc, func(t *testing.T) {
			fmt.Println(tc)
		})
	}
	for _, tc := range []string{"b"} {
		t.Run(tc, func(t *testing.T) {
			fmt.Println(tc)
		})
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	for _, tc := range []string{"a"} {
		tc := tc
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			fmt.Println(tc)
		})
	}
	for _, tc := range []string{"b"} {
		tc := tc
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			fmt.Println(tc)
		})
	}
}
`,
		},
		{
			testCase: "one loop already parallel, the other one is not",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	for _, tc := range []string{"a"} {
		t.Run(tc, func(t *testing.T) {
			fmt.Println(tc)
		})
	}
	for _, tc := range []string{"b"} {
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			fmt.Println(tc)
		})
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	for _, tc := range []string{"a"} {
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			fmt.Println(tc)
		})
	}
	for _, tc := range []string{"b"} {
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			fmt.Println(tc)
		})
	}
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testCase, func(t *testing.T) {
			t.Parallel()

			got, err := GenerateTParallel("./testdata/t/t_test.go", []byte(tt.src), tt.needFixLoopVar)
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != tt.want {
				t.Errorf("result:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
