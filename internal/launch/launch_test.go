package launch

import (
	"reflect"
	"testing"

	"github.com/Smiduweorc/bagworm/internal/config"
	"github.com/Smiduweorc/bagworm/internal/runtime"
)

var (
	podman  = runtime.Runtime{Name: "podman", Path: "/usr/bin/podman", UsernsKeepID: true}
	docker  = runtime.Runtime{Name: "docker", Path: "/usr/bin/docker"}
	nerdctl = runtime.Runtime{Name: "nerdctl", Path: "/usr/bin/nerdctl"}
)

func baseConfig() config.Config {
	return config.Config{
		Image:   "node:22",
		Mounts:  []config.Mount{{Source: "/home/u/proj", Dest: "/workspace"}},
		Workdir: "/workspace",
	}
}

func baseFacts() Facts {
	return Facts{UID: 1000, GID: 1000, TTY: true, Term: "xterm-256color", Home: "/home/u"}
}

// Golden argv tests: the whole point of the pure core.
func TestArgvGolden(t *testing.T) {
	tests := []struct {
		name string
		cfg  func() config.Config
		rt   runtime.Runtime
		f    func() Facts
		want []string
	}{
		{
			name: "podman interactive shell autodetect",
			cfg:  baseConfig,
			rt:   podman,
			f:    baseFacts,
			want: []string{
				"podman", "run", "--rm", "-i", "-t",
				"-v", "/home/u/proj:/workspace",
				"-w", "/workspace",
				"--userns=keep-id",
				"-e", "TERM=xterm-256color",
				"node:22",
				"sh", "-c", ShellFallback,
			},
		},
		{
			name: "docker user mapping and HOME override",
			cfg:  baseConfig,
			rt:   docker,
			f:    baseFacts,
			want: []string{
				"docker", "run", "--rm", "-i", "-t",
				"-v", "/home/u/proj:/workspace",
				"-w", "/workspace",
				"--user", "1000:1000",
				"-e", "HOME=/workspace",
				"-e", "TERM=xterm-256color",
				"node:22",
				"sh", "-c", ShellFallback,
			},
		},
		{
			name: "nerdctl behaves like docker",
			cfg:  baseConfig,
			rt:   nerdctl,
			f:    baseFacts,
			want: []string{
				"nerdctl", "run", "--rm", "-i", "-t",
				"-v", "/home/u/proj:/workspace",
				"-w", "/workspace",
				"--user", "1000:1000",
				"-e", "HOME=/workspace",
				"-e", "TERM=xterm-256color",
				"node:22",
				"sh", "-c", ShellFallback,
			},
		},
		{
			name: "no tty drops -t keeps -i",
			cfg:  baseConfig,
			rt:   podman,
			f: func() Facts {
				f := baseFacts()
				f.TTY = false
				return f
			},
			want: []string{
				"podman", "run", "--rm", "-i",
				"-v", "/home/u/proj:/workspace",
				"-w", "/workspace",
				"--userns=keep-id",
				"-e", "TERM=xterm-256color",
				"node:22",
				"sh", "-c", ShellFallback,
			},
		},
		{
			name: "empty TERM omitted",
			cfg:  baseConfig,
			rt:   podman,
			f: func() Facts {
				f := baseFacts()
				f.Term = ""
				return f
			},
			want: []string{
				"podman", "run", "--rm", "-i", "-t",
				"-v", "/home/u/proj:/workspace",
				"-w", "/workspace",
				"--userns=keep-id",
				"node:22",
				"sh", "-c", ShellFallback,
			},
		},
		{
			name: "explicit command",
			cfg: func() config.Config {
				c := baseConfig()
				c.Command = []string{"bash", "-l"}
				return c
			},
			rt: podman,
			f:  baseFacts,
			want: []string{
				"podman", "run", "--rm", "-i", "-t",
				"-v", "/home/u/proj:/workspace",
				"-w", "/workspace",
				"--userns=keep-id",
				"-e", "TERM=xterm-256color",
				"node:22",
				"bash", "-l",
			},
		},
		{
			name: "multiple mounts preserve order",
			cfg: func() config.Config {
				c := baseConfig()
				c.Mounts = append(c.Mounts, config.Mount{Source: "/home/u/data", Dest: "/data"})
				return c
			},
			rt: podman,
			f:  baseFacts,
			want: []string{
				"podman", "run", "--rm", "-i", "-t",
				"-v", "/home/u/proj:/workspace",
				"-v", "/home/u/data:/data",
				"-w", "/workspace",
				"--userns=keep-id",
				"-e", "TERM=xterm-256color",
				"node:22",
				"sh", "-c", ShellFallback,
			},
		},
		{
			name: "identity mounts podman keep host paths",
			cfg:  baseConfig,
			rt:   podman,
			f: func() Facts {
				f := baseFacts()
				f.IdentityPaths = []string{"/home/u/.gitconfig", "/home/u/.ssh", "/home/u/.config/gh"}
				return f
			},
			want: []string{
				"podman", "run", "--rm", "-i", "-t",
				"-v", "/home/u/proj:/workspace",
				"-v", "/home/u/.gitconfig:/home/u/.gitconfig:ro",
				"-v", "/home/u/.ssh:/home/u/.ssh:ro",
				"-v", "/home/u/.config/gh:/home/u/.config/gh:ro",
				"-w", "/workspace",
				"--userns=keep-id",
				"-e", "TERM=xterm-256color",
				"node:22",
				"sh", "-c", ShellFallback,
			},
		},
		{
			name: "identity mounts docker land under workdir HOME",
			cfg:  baseConfig,
			rt:   docker,
			f: func() Facts {
				f := baseFacts()
				f.IdentityPaths = []string{"/home/u/.gitconfig", "/home/u/.ssh", "/home/u/.config/gh"}
				return f
			},
			want: []string{
				"docker", "run", "--rm", "-i", "-t",
				"-v", "/home/u/proj:/workspace",
				"-v", "/home/u/.gitconfig:/workspace/.gitconfig:ro",
				"-v", "/home/u/.ssh:/workspace/.ssh:ro",
				"-v", "/home/u/.config/gh:/workspace/.config/gh:ro",
				"-w", "/workspace",
				"--user", "1000:1000",
				"-e", "HOME=/workspace",
				"-e", "TERM=xterm-256color",
				"node:22",
				"sh", "-c", ShellFallback,
			},
		},
		{
			name: "selinux enforcing relabels workspace not identity",
			cfg:  baseConfig,
			rt:   podman,
			f: func() Facts {
				f := baseFacts()
				f.SELinuxEnforcing = true
				f.IdentityPaths = []string{"/home/u/.ssh"}
				return f
			},
			want: []string{
				"podman", "run", "--rm", "-i", "-t",
				"-v", "/home/u/proj:/workspace:z",
				"-v", "/home/u/.ssh:/home/u/.ssh:ro",
				"-w", "/workspace",
				"--userns=keep-id",
				"-e", "TERM=xterm-256color",
				"node:22",
				"sh", "-c", ShellFallback,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := Argv(tt.cfg(), tt.rt, tt.f())
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Argv mismatch\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestArgvSELinuxWarning(t *testing.T) {
	f := baseFacts()
	f.SELinuxEnforcing = true
	f.IdentityPaths = []string{"/home/u/.ssh"}
	_, warnings := Argv(baseConfig(), podman, f)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one", warnings)
	}

	// No identity mounts -> nothing to warn about.
	f.IdentityPaths = nil
	_, warnings = Argv(baseConfig(), podman, f)
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none without identity mounts", warnings)
	}
}
