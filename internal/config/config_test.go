package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write creates a file (and parents) under dir and returns its path.
func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFindWalksUpward(t *testing.T) {
	root := t.TempDir()
	want := write(t, root, "bagworm.yaml", "image: node:22\n")
	deep := filepath.Join(root, "src", "components", "deep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, start := range []string{root, deep} {
		got, err := Find(start)
		if err != nil {
			t.Fatalf("Find(%q): %v", start, err)
		}
		// Resolve symlinks: macOS/Linux temp dirs may traverse links.
		gotR, _ := filepath.EvalSymlinks(got)
		wantR, _ := filepath.EvalSymlinks(want)
		if gotR != wantR {
			t.Errorf("Find(%q) = %q, want %q", start, got, want)
		}
	}
}

func TestFindStopsAtNearestConfig(t *testing.T) {
	root := t.TempDir()
	write(t, root, "bagworm.yaml", "image: outer\n")
	inner := write(t, filepath.Join(root, "sub"), "bagworm.yaml", "image: inner\n")

	got, err := Find(filepath.Join(root, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	gotR, _ := filepath.EvalSymlinks(got)
	wantR, _ := filepath.EvalSymlinks(inner)
	if gotR != wantR {
		t.Errorf("Find = %q, want nearest config %q", got, inner)
	}
}

func TestFindNotFoundShowsExample(t *testing.T) {
	_, err := Find(t.TempDir())
	if err == nil {
		t.Fatal("expected error when no config exists")
	}
	for _, want := range []string{"no bagworm.yaml found", "image: node:22"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should contain %q", err, want)
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "bagworm.yaml", "image: node:22\n")

	cfg, err := Load(path, "/home/u")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Image != "node:22" {
		t.Errorf("Image = %q", cfg.Image)
	}
	if cfg.Workdir != "/workspace" {
		t.Errorf("Workdir = %q, want default /workspace", cfg.Workdir)
	}
	if len(cfg.Command) != 0 {
		t.Errorf("Command = %v, want empty", cfg.Command)
	}
	if len(cfg.Mounts) != 1 || cfg.Mounts[0].Dest != "/workspace" {
		t.Fatalf("Mounts = %v, want default .:/workspace", cfg.Mounts)
	}
	if cfg.Mounts[0].Source != filepath.Clean(dir) {
		t.Errorf("default mount source = %q, want project root %q", cfg.Mounts[0].Source, dir)
	}
}

func TestLoadFullConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := write(t, dir, "bagworm.yaml", `image: golang:1.25
mounts:
  - .:/workspace
  - data:/data
workdir: /workspace
command: [bash, -l]
`)
	cfg, err := Load(path, "/home/u")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Mounts) != 2 {
		t.Fatalf("Mounts = %v", cfg.Mounts)
	}
	if cfg.Mounts[1].Source != filepath.Join(dir, "data") || cfg.Mounts[1].Dest != "/data" {
		t.Errorf("relative mount resolved to %+v", cfg.Mounts[1])
	}
	if len(cfg.Command) != 2 || cfg.Command[0] != "bash" || cfg.Command[1] != "-l" {
		t.Errorf("Command = %v", cfg.Command)
	}
}

func TestLoadTildeExpansion(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := write(t, dir, "bagworm.yaml", "image: x\nmounts:\n  - ~/cache:/cache\n")

	cfg, err := Load(path, home)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mounts[0].Source != filepath.Join(home, "cache") {
		t.Errorf("~ expanded to %q", cfg.Mounts[0].Source)
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name, yaml, wantErr string
	}{
		{"missing image", "mounts: ['.:/w']\n", `"image" is required`},
		{"empty image", "image: \"\"\n", `"image" is required`},
		{"unknown field", "image: x\nimagee: y\n", `unknown field "imagee"`},
		{"unknown field line number", "image: x\nbogus: y\n", "line 2"},
		{"relative workdir", "image: x\nworkdir: rel\n", `"workdir" must be an absolute path`},
		{"mount missing dst", "image: x\nmounts: ['./src']\n", `must be "src:dst"`},
		{"mount relative dst", "image: x\nmounts: ['.:workspace']\n", "destination must be an absolute container path"},
		{"mount source missing", "image: x\nmounts: ['./nope:/n']\n", "source does not exist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := write(t, dir, tt.name+".yaml", tt.yaml)
			_, err := Load(path, "/home/u")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadMissingSourceShowsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "bagworm.yaml", "image: x\nmounts: ['./nope:/n']\n")
	_, err := Load(path, "/home/u")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), filepath.Join(dir, "nope")) {
		t.Errorf("error %q should name the absolute source path", err)
	}
}

func TestLoadExplicitEmptyMounts(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "bagworm.yaml", "image: x\nmounts: []\n")
	cfg, err := Load(path, "/home/u")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Mounts) != 0 {
		t.Errorf("explicit empty mounts should stay empty, got %v", cfg.Mounts)
	}
}
