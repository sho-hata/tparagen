package tparagen

import (
	"reflect"
	"testing"
)

func TestProcess(t *testing.T) {
	t.Parallel()
	type args struct {
		filename string
		src      []byte
	}

	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "has not parallel and no range test",
			args: args{
				filename: "./testdata/t/t_test.go",
				src: []byte(`package t

import "testing"

func TestFunctionWithoutTParallelAndRangeTest(t *testing.T) {
	t.Run("hoge", nil)
}`,
				),
			},
			want: []byte(`package t

import "testing"

func TestFunctionWithoutTParallelAndRangeTest(t *testing.T) {
	t.Parallel()
	t.Run("hoge", nil)
}
`),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Process(tt.args.filename, tt.args.src)
			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Process() = \n%v, want\n%v", got, tt.want)
			}
		})
	}
}
