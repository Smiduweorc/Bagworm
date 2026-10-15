// Package trust decides whether a bagworm.yaml may be acted on.
//
// A config chooses the image, the command, and which host paths are mounted,
// and it is found by walking upward from wherever you happen to be. A file
// in a cloned repo, a shared parent directory, or /tmp is not something you
// wrote, so `bagworm enter` will not run one you have not approved.
//
// Approval is recorded per config path together with a hash of its contents:
// editing an approved config (a `git pull` that changes it, say) makes it
// untrusted again until it is re-approved.
package trust

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Store is the set of approved configs, persisted as JSON.
type Store struct {
	path    string
	entries map[string]string // absolute config path -> sha256 of contents
}

// DefaultPath is where the store lives: $XDG_CONFIG_HOME/bagworm/trusted.json
// or its platform equivalent.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating config directory: %w", err)
	}
	return filepath.Join(dir, "bagworm", "trusted.json"), nil
}

// Open loads the store at path. A missing file is an empty store.
func Open(path string) (*Store, error) {
	s := &Store{path: path, entries: make(map[string]string)}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading trust store: %w", err)
	}
	if err := json.Unmarshal(b, &s.entries); err != nil {
		return nil, fmt.Errorf("parsing trust store %s: %w", path, err)
	}
	return s, nil
}

// Trusted reports whether the config at configPath, with these contents, has
// been approved.
func (s *Store) Trusted(configPath string, contents []byte) bool {
	want, ok := s.entries[key(configPath)]
	return ok && want == digest(contents)
}

// Trust approves the config at configPath with these contents and saves.
func (s *Store) Trust(configPath string, contents []byte) error {
	s.entries[key(configPath)] = digest(contents)
	return s.save()
}

// Untrust removes any approval for configPath and saves. It reports whether
// there was one.
func (s *Store) Untrust(configPath string) (bool, error) {
	k := key(configPath)
	if _, ok := s.entries[k]; !ok {
		return false, nil
	}
	delete(s.entries, k)
	return true, s.save()
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("creating trust store directory: %w", err)
	}
	b, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing trust store: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("writing trust store: %w", err)
	}
	return nil
}

// CheckOwnership refuses a config that someone else could have written: one
// owned by another user, or writable by group or others. No approval makes
// such a file safe, since it can change after it is approved.
func CheckOwnership(configPath string) error {
	info, err := os.Stat(configPath)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s is writable by group or others (mode %s); refusing to use it", configPath, info.Mode().Perm())
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s is owned by uid %d, not you (uid %d); refusing to use it", configPath, st.Uid, os.Getuid())
	}
	return nil
}

// key normalizes a config path so the same file is one entry however it was
// reached.
func key(configPath string) string {
	if p, err := filepath.EvalSymlinks(configPath); err == nil {
		configPath = p
	}
	if p, err := filepath.Abs(configPath); err == nil {
		configPath = p
	}
	return configPath
}

func digest(contents []byte) string {
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}
