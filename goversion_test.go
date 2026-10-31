package tparagen

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestNeedFixLoopVar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		v       string
		want    bool
		wantErr bool
	}{
		{v: "1.16", want: true},
		{v: "1.21", want: true},
		{v: "1.21.13", want: true},
		{v: "go1.21", want: true},
		{v: "1.22rc1", want: false},
		{v: "1.22", want: false},
		{v: "1.22.4", want: false},
		{v: "1.26", want: false},
		{v: "1.30", want: false}, // not 1.3
		{v: "", wantErr: true},
		{v: "abc", wantErr: true},
		{v: "1.21.x", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.v, func(t *testing.T) {
			t.Parallel()

			got, err := needFixLoopVar(tt.v)
			if (err != nil) != tt.wantErr {
				t.Fatalf("needFixLoopVar(%q) error = %v, wantErr %v", tt.v, err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("needFixLoopVar(%q) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

func TestParseGoDirective(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "plain", src: "module m\n\ngo 1.21\n", want: "1.21"},
		{name: "patch and comment", src: "module m\n\ngo 1.22.4 // keep\n", want: "1.22.4"},
		{name: "toolchain line is ignored", src: "module m\n\ngo 1.23.0\n\ntoolchain go1.26.2\n", want: "1.23.0"},
		{name: "commented out directive", src: "module m\n\n// go 1.22\n", want: noGoDirectiveVersion},
		{name: "no directive", src: "module m\n", want: noGoDirectiveVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := parseGoDirective([]byte(tt.src)); got != tt.want {
				t.Errorf("parseGoDirective() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGoVersionFromGoModWalksUp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module m\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	v, ok, err := goVersionFromGoMod(nested)
	if err != nil {
		t.Fatal(err)
	}

	if !ok || v != "1.21" {
		t.Errorf("goVersionFromGoMod() = %q, %v; want \"1.21\", true", v, ok)
	}
}

func TestNeedFixLoopVarIn(t *testing.T) {
	t.Parallel()

	// root (go 1.22) contains old (go 1.21), a module of its own, and plain,
	// a package of root.
	root := t.TempDir()
	old := filepath.Join(root, "old", "pkg")
	plain := filepath.Join(root, "plain")

	for _, dir := range []string{old, plain} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module m\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "old", "go.mod"), []byte("module m/old\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		minGoVersion string
		dir          string
		want         bool
		wantErr      bool
	}{
		{name: "detected from go.mod", dir: plain, want: false},
		{name: "a nested module has its own version", dir: old, want: true},
		{name: "explicit version wins over go.mod", minGoVersion: "1.21", dir: plain, want: true},
		{name: "explicit version wins over a nested go.mod", minGoVersion: "1.22", dir: old, want: false},
		{name: "invalid explicit version", minGoVersion: "bogus", dir: plain, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tp := &tparagen{in: root, errStream: io.Discard, minGoVersion: tt.minGoVersion}

			got, err := tp.needFixLoopVarIn(tt.dir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("needFixLoopVarIn(%q) = %v, want %v", tt.dir, got, tt.want)
			}
		})
	}
}

func TestParseModulePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "plain", src: "module example.com/m\n\ngo 1.21\n", want: "example.com/m"},
		{name: "quoted with comment", src: "module \"example.com/m\" // c\n", want: "example.com/m"},
		{name: "commented out", src: "// module example.com/m\n", want: ""},
		{name: "block", src: "module (\n\t// c\n\texample.com/m\n)\n\ngo 1.21\n", want: "example.com/m"},
		{name: "empty block", src: "module (\n)\n", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := parseModulePath([]byte(tt.src)); got != tt.want {
				t.Errorf("parseModulePath() = %q, want %q", got, tt.want)
			}
		})
	}
}
