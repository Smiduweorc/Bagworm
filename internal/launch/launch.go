// Package launch assembles the runtime argv. This is the pure core of
// Bagworm: (config, runtime, host facts) -> []string, with no side effects.
// Everything observable about the host is passed in via Facts so the
// assembly is deterministic and golden-testable.
package launch

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Smiduweorc/bagworm/internal/config"
	"github.com/Smiduweorc/bagworm/internal/runtime"
)

// ShellFallback is used when the config sets no command: one container
// start, bash if the image has it, sh otherwise, exec so no wrapper
// process is left behind.
const ShellFallback = "command -v bash >/dev/null 2>&1 && exec bash || exec sh"

// Facts are the host-side observations argv assembly depends on. Gathering
// them (the side effects) lives in internal/enter.
type Facts struct {
	UID int
	GID int

	// TTY is whether stdout is a terminal; it gates -t. -i is always set.
	TTY bool

	// Term is the host $TERM, forwarded into the container when non-empty.
	Term string

	// Home is the host home directory, used to map identity mounts.
	Home string

	// SELinuxEnforcing adds :z to workspace mounts and downgrades identity
	// mounts to a warning (relabeling ~/.ssh would be harmful).
	SELinuxEnforcing bool

	// IdentityPaths are the identity files/dirs that exist on the host
	// (~/.gitconfig, ~/.ssh, ~/.config/gh), as absolute paths.
	IdentityPaths []string
}

// Argv builds the full runtime command line. argv[0] is the runtime name,
// suitable for syscall.Exec together with runtime.Path. warnings are
// human-readable notes for stderr; they never abort the launch.
func Argv(cfg config.Config, rt runtime.Runtime, f Facts) (argv []string, warnings []string) {
	argv = []string{rt.Name, "run", "--rm", "-i"}
	if f.TTY {
		argv = append(argv, "-t")
	}

	// Project mounts. Under enforcing SELinux plain -v mounts are
	// unreadable, so relabel workspace mounts with :z.
	for _, m := range cfg.Mounts {
		spec := m.Source + ":" + m.Dest
		if f.SELinuxEnforcing {
			spec += ":z"
		}
		argv = append(argv, "-v", spec)
	}

	// Identity mounts: read-only, only those that exist on the host, and
	// never relabeled - instead warn that SELinux may block them.
	if f.SELinuxEnforcing && len(f.IdentityPaths) > 0 {
		warnings = append(warnings,
			"SELinux is enforcing: identity mounts (git/ssh config) may be unreadable inside the container")
	}
	for _, p := range f.IdentityPaths {
		argv = append(argv, "-v", p+":"+identityDest(p, cfg.Workdir, f.Home, rt)+":ro")
	}

	argv = append(argv, "-w", cfg.Workdir)

	// User mapping - the per-runtime 5%. Rootless podman keeps the host
	// UID/GID with a real /etc/passwd entry; docker/nerdctl get the raw
	// uid:gid plus a writable HOME so git/ssh/tools still work.
	if rt.UsernsKeepID {
		argv = append(argv, "--userns=keep-id")
	} else {
		argv = append(argv,
			"--user", fmt.Sprintf("%d:%d", f.UID, f.GID),
			"-e", "HOME="+cfg.Workdir)
	}

	if f.Term != "" {
		argv = append(argv, "-e", "TERM="+f.Term)
	}

	argv = append(argv, cfg.Image)

	if len(cfg.Command) > 0 {
		argv = append(argv, cfg.Command...)
	} else {
		argv = append(argv, "sh", "-c", ShellFallback)
	}
	return argv, warnings
}

// identityDest maps a host identity path to its container destination:
// the same path relative to $HOME, placed under the container home. For
// podman with keep-id the container home is the host home; for
// docker/nerdctl it is the workdir (we set HOME=<workdir>).
func identityDest(hostPath, workdir, home string, rt runtime.Runtime) string {
	if rt.UsernsKeepID {
		return hostPath
	}
	rel, err := filepath.Rel(home, hostPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		// Not under home (shouldn't happen for identity paths); keep the
		// host path as-is rather than guessing.
		return hostPath
	}
	return filepath.Join(workdir, rel)
}
