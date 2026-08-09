//go:build integration

// Integration tests: require a real container runtime and network access
// to pull small images. Run with:
//
//	go test -tags integration ./internal/enter/
//
// Scenarios (PLAN.md milestone 8): enter + exit code, bash-vs-sh
// fallback, and a file-ownership round-trip.
package enter_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Smiduweorc/bagworm/internal/runtime"
)

// buildBinary compiles bagworm once per test run.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bagworm")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/Smiduweorc/bagworm/cmd/bagworm")
	cmd.Dir = projectRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func projectRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(strings.TrimSpace(string(out)))
}

// project creates a temp project dir with the given bagworm.yaml.
func project(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bagworm.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// enter runs `bagworm enter` in dir with stdin piped (no TTY, so no -t).
func enter(t *testing.T, bin, dir, stdin string) (stdout string, exitCode int) {
	t.Helper()
	cmd := exec.Command(bin, "enter")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running bagworm enter: %v\nstderr: %s", err, errb.String())
	}
	t.Logf("stderr: %s", errb.String())
	return out.String(), code
}

func requireRuntime(t *testing.T) {
	t.Helper()
	rt, err := runtime.Detect()
	if err != nil {
		t.Skip("no container runtime installed")
	}
	if err := exec.Command(rt.Path, "info").Run(); err != nil {
		t.Skipf("%s is installed but not usable (daemon down / no permission): %v", rt.Name, err)
	}
}

func TestEnterExitCode(t *testing.T) {
	requireRuntime(t)
	bin := buildBinary(t)
	dir := project(t, "image: alpine:3\n")

	_, code := enter(t, bin, dir, "exit 7\n")
	if code != 7 {
		t.Errorf("exit code = %d, want 7 (should pass through from the container shell)", code)
	}
}

func TestShellFallback(t *testing.T) {
	requireRuntime(t)
	bin := buildBinary(t)

	t.Run("alpine falls back to sh", func(t *testing.T) {
		dir := project(t, "image: alpine:3\n")
		out, code := enter(t, bin, dir, "echo shell=$0\n")
		if code != 0 {
			t.Fatalf("exit code = %d", code)
		}
		if !strings.Contains(out, "shell=sh") && !strings.Contains(out, "shell=/bin/sh") {
			t.Errorf("output = %q, want sh as the shell", out)
		}
	})

	t.Run("debian picks bash", func(t *testing.T) {
		dir := project(t, "image: debian:stable-slim\n")
		out, code := enter(t, bin, dir, "echo bash=$BASH_VERSION\n")
		if code != 0 {
			t.Fatalf("exit code = %d", code)
		}
		if strings.Contains(out, "bash=\n") || strings.TrimSpace(out) == "bash=" {
			t.Errorf("output = %q, want a non-empty BASH_VERSION", out)
		}
	})
}

func TestFileOwnershipRoundTrip(t *testing.T) {
	requireRuntime(t)
	bin := buildBinary(t)
	dir := project(t, "image: alpine:3\n")

	_, code := enter(t, bin, dir, "touch /workspace/created-inside\n")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}

	info, err := os.Stat(filepath.Join(dir, "created-inside"))
	if err != nil {
		t.Fatalf("file created in container not visible on host: %v", err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	if int(stat.Uid) != os.Getuid() || int(stat.Gid) != os.Getgid() {
		t.Errorf("file owned by %d:%d, want %d:%d (user mapping broken)",
			stat.Uid, stat.Gid, os.Getuid(), os.Getgid())
	}
}
