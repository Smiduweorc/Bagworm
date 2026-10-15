package enter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Smiduweorc/bagworm/internal/config"
)

func TestConfirm(t *testing.T) {
	for answer, want := range map[string]bool{
		"y\n": true, "yes\n": true, "Y\n": true,
		"\n": false, "n\n": false, "": false, "yep\n": false,
	} {
		var out strings.Builder
		got := confirm(strings.NewReader(answer), &out, "/p/bagworm.yaml", []byte("image: evil\nidentity: [ssh]\n"))
		if got != want {
			t.Errorf("answer %q: confirm = %v, want %v", answer, got, want)
		}
		// The prompt shows what is being approved.
		if !strings.Contains(out.String(), "| identity: [ssh]") {
			t.Errorf("prompt does not show the config:\n%s", out.String())
		}
	}
}

func TestGatherFactsOnlyOptedInIdentities(t *testing.T) {
	home := t.TempDir()
	for _, rel := range []string{".gitconfig", ".ssh/id_ed25519", ".config/gh/hosts.yml"} {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if got := gatherFacts(home, nil).IdentityPaths; len(got) != 0 {
		t.Errorf("no identities asked for, got %v", got)
	}

	got := gatherFacts(home, []config.Identity{config.IdentityGit}).IdentityPaths
	if len(got) != 1 || got[0] != filepath.Join(home, ".gitconfig") {
		t.Errorf("git only: got %v", got)
	}

	// Asked for but absent on the host: skipped, not an error.
	empty := t.TempDir()
	if got := gatherFacts(empty, []config.Identity{config.IdentitySSH}).IdentityPaths; len(got) != 0 {
		t.Errorf("missing ~/.ssh: got %v", got)
	}
}
