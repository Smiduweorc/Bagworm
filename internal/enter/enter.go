// Package enter glues the pipeline together:
//
//	find config -> parse/validate -> detect runtime -> gather host facts
//	    -> assemble argv -> (--dry-run? print : syscall.Exec)
//
// The final syscall.Exec replaces the bagworm process entirely, so
// signals, TTY resize, and exit codes are the runtime's problem - which
// is exactly where they Just Work.
package enter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/Smiduweorc/bagworm/internal/config"
	"github.com/Smiduweorc/bagworm/internal/launch"
	"github.com/Smiduweorc/bagworm/internal/runtime"
)

// Options controls a single `bagworm enter` invocation.
type Options struct {
	// DryRun prints the assembled command (shell-quoted, one argument per
	// line) instead of executing it.
	DryRun bool
}

// Run executes the enter pipeline. On a real (non-dry) run it does not
// return on success: the process is replaced by the runtime CLI.
func Run(opts Options) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	path, err := config.Find(cwd)
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("determining home directory: %w", err)
	}

	cfg, err := config.Load(path, home)
	if err != nil {
		return err
	}

	rt, err := runtime.Detect()
	if err != nil {
		return err
	}

	argv, warnings := launch.Argv(cfg, rt, gatherFacts(home))
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning: "+w)
	}

	if opts.DryRun {
		fmt.Println(formatArgv(argv))
		return nil
	}

	// Point of no return: bagworm ceases to exist.
	if err := syscall.Exec(rt.Path, argv, os.Environ()); err != nil {
		return fmt.Errorf("executing %s: %w", rt.Path, err)
	}
	return nil // unreachable
}

// gatherFacts collects every host-side observation the pure argv assembly
// needs. All side effects up to the final exec live here and in config.
func gatherFacts(home string) launch.Facts {
	f := launch.Facts{
		UID:              os.Getuid(),
		GID:              os.Getgid(),
		TTY:              term.IsTerminal(int(os.Stdout.Fd())),
		Term:             os.Getenv("TERM"),
		Home:             home,
		SELinuxEnforcing: selinuxEnforcing(),
	}
	for _, rel := range []string{".gitconfig", ".ssh", filepath.Join(".config", "gh")} {
		p := filepath.Join(home, rel)
		if _, err := os.Stat(p); err == nil {
			f.IdentityPaths = append(f.IdentityPaths, p)
		}
	}
	return f
}

// selinuxEnforcing reports whether SELinux is present and enforcing.
// Hosts without SELinux (e.g. Arch by default) simply lack the file.
func selinuxEnforcing() bool {
	b, err := os.ReadFile("/sys/fs/selinux/enforce")
	return err == nil && strings.TrimSpace(string(b)) == "1"
}

// formatArgv renders the command shell-quoted, one argument per line with
// continuations, so the dry-run output is copy-pasteable.
func formatArgv(argv []string) string {
	lines := make([]string, len(argv))
	for i, a := range argv {
		q := shellQuote(a)
		switch {
		case i == 0:
			lines[i] = q + " \\"
		case i == len(argv)-1:
			lines[i] = "  " + q
		default:
			lines[i] = "  " + q + " \\"
		}
	}
	return strings.Join(lines, "\n")
}

// shellQuote single-quotes an argument when it contains anything a POSIX
// shell would interpret.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\"'`$&|;<>()*?[]#~%!{}\\") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
