package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBin drops an executable stub named name into dir.
func fakeBin(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestDetectPriorityOrder(t *testing.T) {
	tests := []struct {
		name      string
		installed []string
		want      string
	}{
		{"podman wins over all", []string{"podman", "docker", "nerdctl"}, "podman"},
		{"docker over nerdctl", []string{"docker", "nerdctl"}, "docker"},
		{"nerdctl alone", []string{"nerdctl"}, "nerdctl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, bin := range tt.installed {
				fakeBin(t, dir, bin)
			}
			t.Setenv("PATH", dir)

			rt, err := Detect()
			if err != nil {
				t.Fatal(err)
			}
			if rt.Name != tt.want {
				t.Errorf("Detect().Name = %q, want %q", rt.Name, tt.want)
			}
			if rt.Path != filepath.Join(dir, tt.want) {
				t.Errorf("Detect().Path = %q", rt.Path)
			}
			if rt.UsernsKeepID != (tt.want == "podman") {
				t.Errorf("UsernsKeepID = %v for %s", rt.UsernsKeepID, rt.Name)
			}
		})
	}
}

func TestDetectNoneFound(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := Detect()
	if err == nil {
		t.Fatal("expected error with empty PATH")
	}
	for _, want := range []string{"podman", "docker", "nerdctl"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should name %s", err, want)
		}
	}
}
