package tparagen

import (
	"reflect"
	"testing"
)

func TestProcess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testCase string
		filename string
		src      string
		want     string
		wantErr  bool
	}{
		{
			testCase: "do not called t.Parallel in main test",
			filename: "./testdata/t/t_test.go",
			src: `package t

import "testing"

func TestFunctionDoNotCalledParallelInMain(t *testing.T) {
	t.Run("hoge", nil)
}`,
			want: `package t

import "testing"

func TestFunctionDoNotCalledParallelInMain(t *testing.T) {
	t.Parallel()
	t.Run("hoge", nil)
}
`,
			wantErr: false,
		},
		{
			testCase: "called t.Parallel in main test",
			filename: "./testdata/t/t_test.go",
			src: `package t

import "testing"

func TestFunctionCalledParallelInMain(t *testing.T) {
	t.Parallel()
	t.Run("hoge", nil)
}`,
			want: `package t

import "testing"

func TestFunctionCalledParallelInMain(t *testing.T) {
	t.Parallel()
	t.Run("hoge", nil)
}
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testCase, func(t *testing.T) {
			t.Parallel()
			got, err := Process(tt.filename, []byte(tt.src))
			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !reflect.DeepEqual(got, []byte(tt.want)) {
				t.Errorf("Process() = \n%v, want\n%v", string(got), tt.want)
			}
		})
	}
}
