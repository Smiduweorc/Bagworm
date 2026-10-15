package trust

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrustIsPerContents(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "bagworm.yaml")
	store, err := Open(filepath.Join(dir, "state", "trusted.json"))
	if err != nil {
		t.Fatal(err)
	}

	v1 := []byte("image: alpine\n")
	if store.Trusted(cfg, v1) {
		t.Fatal("a config nobody approved is trusted")
	}
	if err := store.Trust(cfg, v1); err != nil {
		t.Fatal(err)
	}
	if !store.Trusted(cfg, v1) {
		t.Fatal("approved config is not trusted")
	}

	// Any edit withdraws the approval: the image or the mounts may now be
	// something else entirely.
	if store.Trusted(cfg, []byte("image: alpine\nidentity: [ssh]\n")) {
		t.Error("an edited config is still trusted")
	}
	// Approval is for that file, not those bytes anywhere.
	if store.Trusted(filepath.Join(dir, "other", "bagworm.yaml"), v1) {
		t.Error("approval leaked to a different path")
	}
}

func TestStorePersists(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "state", "trusted.json")
	cfg := filepath.Join(dir, "bagworm.yaml")
	data := []byte("image: alpine\n")

	s, _ := Open(storePath)
	if err := s.Trust(cfg, data); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("store mode = %v, want 0600", perm)
	}

	reopened, err := Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Trusted(cfg, data) {
		t.Error("approval did not survive a reopen")
	}

	removed, err := reopened.Untrust(cfg)
	if err != nil || !removed {
		t.Fatalf("Untrust = %v, %v; want true, nil", removed, err)
	}
	again, _ := Open(storePath)
	if again.Trusted(cfg, data) {
		t.Error("untrusted config is still trusted after reopen")
	}
	if removed, _ := again.Untrust(cfg); removed {
		t.Error("Untrust of an unknown config reported a removal")
	}
}

func TestTrustFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.yaml")
	link := filepath.Join(dir, "link.yaml")
	if err := os.WriteFile(real, []byte("image: a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	s, _ := Open(filepath.Join(dir, "trusted.json"))
	if err := s.Trust(link, []byte("image: a\n")); err != nil {
		t.Fatal(err)
	}
	if !s.Trusted(real, []byte("image: a\n")) {
		t.Error("the same file reached two ways is two entries")
	}
}

func TestOpenRejectsCorruptStore(t *testing.T) {
	p := filepath.Join(t.TempDir(), "trusted.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(p); err == nil {
		t.Error("a corrupt store opened without error")
	}
}

func TestCheckOwnership(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bagworm.yaml")
	if err := os.WriteFile(p, []byte("image: a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckOwnership(p); err != nil {
		t.Errorf("own 0644 file refused: %v", err)
	}

	for _, mode := range []os.FileMode{0o664, 0o646} {
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		err := CheckOwnership(p)
		if err == nil || !strings.Contains(err.Error(), "writable by group or others") {
			t.Errorf("mode %v: err = %v, want a refusal", mode, err)
		}
	}
}
