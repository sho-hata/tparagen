package sample

import "testing"

func TestAdd(t *testing.T) {
	tests := []struct {
		name       string
		a, b, want int
	}{
		{"one", 1, 1, 2},
		{"two", 2, 2, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a + tt.b; got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestConfig(t *testing.T) {
	t.Setenv("APP_ENV", "test")
}
