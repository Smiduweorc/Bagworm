// Package runtime detects which OCI container runtime is installed.
//
// Runtimes are an implementation detail: nothing user-facing ever requires
// knowing which one was picked. The three supported CLIs are ~95%
// argv-compatible, so the differences are modeled as data on the Runtime
// struct, not as an interface hierarchy.
package runtime

import (
	"fmt"
	"os"
	"os/exec"
)

// Runtime describes the detected container CLI plus its quirks.
type Runtime struct {
	Name string // "podman", "docker", or "nerdctl"
	Path string // absolute path from exec.LookPath

	// UsernsKeepID: rootless podman maps the host user into the container
	// with --userns=keep-id. Docker and nerdctl instead need an explicit
	// --user <uid>:<gid> plus a HOME override (see internal/launch).
	UsernsKeepID bool
}

// candidates in fixed priority order. First hit wins; no flags, no config,
// no env override in v0.1.
var candidates = []Runtime{
	{Name: "podman", UsernsKeepID: true},
	{Name: "docker"},
	{Name: "nerdctl"},
}

// Detect finds the first installed runtime in priority order
// (podman -> docker -> nerdctl).
func Detect() (Runtime, error) {
	for _, c := range candidates {
		if path, err := exec.LookPath(c.Name); err == nil {
			c.Path = path
			return c, nil
		}
	}
	return Runtime{}, fmt.Errorf(
		"no container runtime found: looked for podman, docker, nerdctl (in that order) in PATH:\n  %s",
		os.Getenv("PATH"))
}
