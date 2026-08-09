// Package config finds, parses, and validates bagworm.yaml.
//
// The file is discovered git-style: walking upward from the current
// directory to the filesystem root. The directory containing the config
// becomes the project root, which relative mount sources resolve against.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileName is the only config filename Bagworm looks for. One name, no
// ambiguity (see PLAN.md, open decision 1).
const FileName = "bagworm.yaml"

// minimalExample is shown (pre-indented) when no config is found, so the
// error message itself is copy-pasteable.
const minimalExample = `  # bagworm.yaml
  image: node:22`

// Mount is a single bind mount with the source already resolved to an
// absolute host path.
type Mount struct {
	Source string // absolute host path
	Dest   string // absolute container path
}

// Config is the validated, resolved form of bagworm.yaml.
type Config struct {
	Image   string
	Mounts  []Mount
	Workdir string
	Command []string // empty means "autodetect a shell at launch"
}

// raw mirrors the YAML schema exactly. Decoding is strict, so any field
// not listed here is a hard error.
type raw struct {
	Image   string   `yaml:"image"`
	Mounts  []string `yaml:"mounts"`
	Workdir string   `yaml:"workdir"`
	Command []string `yaml:"command"`
}

// Find walks upward from startDir to the filesystem root looking for
// bagworm.yaml and returns its absolute path.
func Find(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, FileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	start, _ := filepath.Abs(startDir)
	return "", fmt.Errorf(
		"no %s found (searched from %s up to /)\n\ncreate one in your project root:\n\n%s",
		FileName, start, minimalExample)
}

// Load parses and validates the config at path. home is the host home
// directory, used to expand ~ in mount sources.
func Load(path, home string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}
	defer f.Close()

	var r raw
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&r); err != nil {
		return Config{}, fmt.Errorf("parsing %s: %s", path, friendlyYAMLError(err))
	}

	root := filepath.Dir(path)
	cfg := Config{
		Image:   r.Image,
		Workdir: r.Workdir,
		Command: r.Command,
	}

	if strings.TrimSpace(cfg.Image) == "" {
		return Config{}, fmt.Errorf("%s: \"image\" is required and must not be empty", path)
	}

	if cfg.Workdir == "" {
		cfg.Workdir = "/workspace"
	}
	if !strings.HasPrefix(cfg.Workdir, "/") {
		return Config{}, fmt.Errorf("%s: \"workdir\" must be an absolute path, got %q", path, cfg.Workdir)
	}

	rawMounts := r.Mounts
	if rawMounts == nil {
		rawMounts = []string{".:/workspace"}
	}
	for _, m := range rawMounts {
		mount, err := resolveMount(m, root, home)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", path, err)
		}
		cfg.Mounts = append(cfg.Mounts, mount)
	}

	return cfg, nil
}

// resolveMount parses a "src:dst" entry, expands the source against the
// project root or $HOME, and verifies it exists on the host.
func resolveMount(entry, root, home string) (Mount, error) {
	parts := strings.Split(entry, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Mount{}, fmt.Errorf("mount %q: must be \"src:dst\"", entry)
	}
	src, dst := parts[0], parts[1]

	if !strings.HasPrefix(dst, "/") {
		return Mount{}, fmt.Errorf("mount %q: destination must be an absolute container path, got %q", entry, dst)
	}

	switch {
	case src == "~":
		src = home
	case strings.HasPrefix(src, "~/"):
		src = filepath.Join(home, src[2:])
	case !filepath.IsAbs(src):
		src = filepath.Join(root, src)
	}
	src = filepath.Clean(src)

	if _, err := os.Stat(src); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Mount{}, fmt.Errorf("mount %q: source does not exist: %s", entry, src)
		}
		return Mount{}, fmt.Errorf("mount %q: checking source %s: %w", entry, src, err)
	}

	return Mount{Source: src, Dest: filepath.Clean(dst)}, nil
}

// unknownField matches yaml.v3's strict-mode error lines, e.g.
// "line 4: field imagee not found in type config.raw".
var unknownField = regexp.MustCompile(`line (\d+): field (\S+) not found in type \S+`)

// friendlyYAMLError rewrites yaml.v3's strict-decoding errors so users see
// "unknown field" with the key and line number instead of Go type names.
func friendlyYAMLError(err error) string {
	var typeErr *yaml.TypeError
	if !errors.As(err, &typeErr) {
		return err.Error()
	}
	msgs := make([]string, 0, len(typeErr.Errors))
	for _, e := range typeErr.Errors {
		if m := unknownField.FindStringSubmatch(e); m != nil {
			msgs = append(msgs, fmt.Sprintf("line %s: unknown field %q (valid fields: image, mounts, workdir, command)", m[1], m[2]))
		} else {
			msgs = append(msgs, e)
		}
	}
	return strings.Join(msgs, "\n")
}
