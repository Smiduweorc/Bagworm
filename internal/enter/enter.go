// Package enter glues the pipeline together:
//
//	find config -> check trust -> parse/validate -> detect runtime
//	    -> gather host facts -> assemble argv -> (--dry-run? print : syscall.Exec)
//
// The final syscall.Exec replaces the bagworm process entirely, so
// signals, TTY resize, and exit codes are the runtime's problem - which
// is exactly where they Just Work.
package enter

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/Smiduweorc/bagworm/internal/config"
	"github.com/Smiduweorc/bagworm/internal/launch"
	"github.com/Smiduweorc/bagworm/internal/runtime"
	"github.com/Smiduweorc/bagworm/internal/trust"
)

// Options controls a single `bagworm enter` invocation.
type Options struct {
	// DryRun prints the assembled command (shell-quoted, one argument per
	// line) instead of executing it.
	DryRun bool

	// NoIdentity skips every identity mount, whatever the config asks for.
	NoIdentity bool
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

	data, err := readChecked(path)
	if err != nil {
		return err
	}

	cfg, err := config.Parse(path, data, home)
	if err != nil {
		return err
	}

	if opts.NoIdentity {
		cfg.Identity = nil
	}

	// A dry run executes nothing, so it is how you look at an untrusted
	// config before approving it.
	if !opts.DryRun {
		if err := requireTrust(path, data); err != nil {
			return err
		}
	}

	rt, err := runtime.Detect()
	if err != nil {
		return err
	}

	argv, warnings := launch.Argv(cfg, rt, gatherFacts(home, cfg.Identity))
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning: "+w)
	}

	if opts.DryRun {
		if !trusted(path, data) {
			fmt.Fprintf(os.Stderr, "note: %s is not trusted yet; `bagworm trust` approves it\n", path)
		}
		fmt.Println(formatArgv(argv))
		return nil
	}

	// Point of no return: bagworm ceases to exist.
	if err := syscall.Exec(rt.Path, argv, os.Environ()); err != nil {
		return fmt.Errorf("executing %s: %w", rt.Path, err)
	}
	return nil // unreachable
}

// readChecked reads a config after refusing one that another user could have
// written.
func readChecked(path string) ([]byte, error) {
	if err := trust.CheckOwnership(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}

// requireTrust lets an approved config through, and otherwise shows it and
// asks. The question goes to the terminal rather than stdin, which belongs to
// the container (`bagworm enter <<< 'make test'`); with no terminal there is
// no one to ask, so it fails and says how to approve the file.
func requireTrust(path string, data []byte) error {
	storePath, err := trust.DefaultPath()
	if err != nil {
		return err
	}
	store, err := trust.Open(storePath)
	if err != nil {
		return err
	}
	if store.Trusted(path, data) {
		return nil
	}

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("%s is not trusted (new, or changed since it was approved)\n\n"+
			"review it, then approve it with:\n\n  bagworm trust %s", path, path)
	}
	defer tty.Close()

	if !confirm(tty, tty, path, data) {
		return fmt.Errorf("not entering: %s was not approved", path)
	}
	return store.Trust(path, data)
}

// trusted reports whether the config is approved, treating an unreadable
// store as "no".
func trusted(path string, data []byte) bool {
	storePath, err := trust.DefaultPath()
	if err != nil {
		return false
	}
	store, err := trust.Open(storePath)
	return err == nil && store.Trusted(path, data)
}

// confirm shows the config and asks whether to trust it. Only "y" or "yes"
// approves.
func confirm(in io.Reader, out io.Writer, path string, data []byte) bool {
	fmt.Fprintf(out, "%s is not trusted (new, or changed since it was approved).\n", path)
	fmt.Fprintf(out, "It picks the image, the command, and the host paths mounted:\n\n")
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		fmt.Fprintf(out, "  | %s\n", line)
	}
	fmt.Fprintf(out, "\nTrust it and enter? [y/N] ")
	answer, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	}
	return false
}

// Trust approves the config that `bagworm enter` would use from dir, as it
// is now.
func Trust(dir string) (string, error) {
	path, err := config.Find(dir)
	if err != nil {
		return "", err
	}
	data, err := readChecked(path)
	if err != nil {
		return "", err
	}
	storePath, err := trust.DefaultPath()
	if err != nil {
		return "", err
	}
	store, err := trust.Open(storePath)
	if err != nil {
		return "", err
	}
	return path, store.Trust(path, data)
}

// Untrust withdraws approval for the config `bagworm enter` would use from
// dir. It reports whether there was an approval to withdraw.
func Untrust(dir string) (string, bool, error) {
	path, err := config.Find(dir)
	if err != nil {
		return "", false, err
	}
	storePath, err := trust.DefaultPath()
	if err != nil {
		return "", false, err
	}
	store, err := trust.Open(storePath)
	if err != nil {
		return "", false, err
	}
	removed, err := store.Untrust(path)
	return path, removed, err
}

// gatherFacts collects every host-side observation the pure argv assembly
// needs. All side effects up to the final exec live here and in config.
// Only the identities the config opted into are considered.
func gatherFacts(home string, identities []config.Identity) launch.Facts {
	f := launch.Facts{
		UID:              os.Getuid(),
		GID:              os.Getgid(),
		TTY:              term.IsTerminal(int(os.Stdout.Fd())),
		Term:             os.Getenv("TERM"),
		Home:             home,
		SELinuxEnforcing: selinuxEnforcing(),
	}
	for _, id := range identities {
		p := filepath.Join(home, config.IdentityPaths[id])
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
