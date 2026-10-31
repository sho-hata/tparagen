package tparagen

import "testing"

// A parallel subtest only starts after the function that started it has
// returned, so after that function's deferred calls have run. Subtests that use
// what a defer statement closes, cancels or waits for stay sequential.
func TestGenerateTParallelDeferredValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase       string
		src            string
		want           string
		needFixLoopVar bool
	}{
		{
			testCase: "subtest using a deferred value stays sequential",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	srv := httptest.NewServer(h)
	defer srv.Close()

	t.Run("1", func(t *testing.T) {
		get(t, srv.URL)
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(h)
	defer srv.Close()

	t.Run("1", func(t *testing.T) {
		get(t, srv.URL)
	})
}
`,
		},
		{
			testCase: "a variable declared together with a deferred one counts",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			call(ctx, tc)
		})
	}
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			call(ctx, tc)
		})
	}
}
`,
			needFixLoopVar: true,
		},
		{
			testCase: "a deferred function literal counts",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	var wg sync.WaitGroup
	defer func() {
		wg.Wait()
	}()

	t.Run("1", func(t *testing.T) {
		wg.Add(1)
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	defer func() {
		wg.Wait()
	}()

	t.Run("1", func(t *testing.T) {
		wg.Add(1)
	})
}
`,
		},
		{
			testCase: "a defer that the subtests do not use does not matter",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	f := open(t)
	defer f.Close()

	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	f := open(t)
	defer f.Close()

	t.Run("1", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})
}
`,
		},
		{
			testCase: "passing t to a defer does not count",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	be := newBackend(t)
	defer closeBackend(t, be)

	t.Run("1", func(t *testing.T) {
		fmt.Println("1")
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	be := newBackend(t)
	defer closeBackend(t, be)

	t.Run("1", func(t *testing.T) {
		t.Parallel()
		fmt.Println("1")
	})
}
`,
		},
		{
			testCase: "t.Cleanup runs after parallel subtests and does not count",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	t.Run("1", func(t *testing.T) {
		get(t, srv.URL)
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	t.Run("1", func(t *testing.T) {
		t.Parallel()
		get(t, srv.URL)
	})
}
`,
		},
		{
			testCase: "a defer in a subtest affects only its own subtests",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	t.Run("outer", func(t *testing.T) {
		srv := httptest.NewServer(h)
		defer srv.Close()

		t.Run("inner", func(t *testing.T) {
			get(t, srv.URL)
		})
	})
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	t.Run("outer", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(h)
		defer srv.Close()

		t.Run("inner", func(t *testing.T) {
			get(t, srv.URL)
		})
	})
}
`,
		},
		{
			testCase: "a defer in a nested function literal runs when that literal returns",
			src: `package t

import "testing"

func TestX(t *testing.T) {
	srv := httptest.NewServer(h)

	func() {
		defer srv.Close()
	}()

	run := func(name string) {
		conn := dial(srv.URL)
		defer conn.Close()

		t.Run(name, func(t *testing.T) {
			use(conn)
		})
	}

	run("1")
}
`,
			want: `package t

import "testing"

func TestX(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(h)

	func() {
		defer srv.Close()
	}()

	run := func(name string) {
		conn := dial(srv.URL)
		defer conn.Close()

		t.Run(name, func(t *testing.T) {
			use(conn)
		})
	}

	run("1")
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
