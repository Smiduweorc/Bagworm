![Logo](./assets/logo.png)

# bagworm

```sh
$ bagworm enter
```

Just create a simple `bagworm.yml` file and run your project. This utility skips the need to configure a workspace if you just need a temporary shell.

## Quick start

1. Drop a `bagworm.yaml` in your project root. Only `image` is required:

   ```yaml
   # bagworm.yaml
   image: node:22
   ```

2. Enter, from anywhere in the project:

   ```sh
   $ cd deep/inside/your/project
   $ bagworm enter
   node@workspace:/workspace$ 
   ```

Files you create inside are owned by you on the host. Exit the shell and the
container is gone (`--rm` always - state lives in the mounts, not the
container).

## Configuration

The complete v0.1 schema - this is all of it, and that's the point:

```yaml
# bagworm.yaml
image: node:22          # required: the OCI image to inhabit

mounts:                 # default: [".:/workspace"]
  - .:/workspace        # relative sources resolve against the project root
  - ~/data:/data        # ~ expands to your home; destinations must be absolute

workdir: /workspace     # default: /workspace

command:                # default: autodetect - bash if the image has it, sh otherwise
  - bash
```

The project root is wherever `bagworm.yaml` lives (found git-style, walking
upward), so `bagworm enter` works from any subdirectory. A typo in the config
is a hard error that names the offending key and line number, rather than a
silent no-op.

See [`bagworm.example.yaml`](./bagworm.example.yaml) for the annotated version.

## Everyday use

Everything below is still just `bagworm enter`. The container shell reads
stdin, so anything you can type interactively you can also pipe in.

A working session - build inside, own the results outside:

```sh
$ bagworm enter
node@workspace:/workspace$ node --version
v22.17.0
node@workspace:/workspace$ npm run build
node@workspace:/workspace$ exit
$ ls -l dist/app.js
-rw-r--r-- 1 you you 48210 Jul  4 14:12 dist/app.js
```

The artifact belongs to you on the host, because you were you inside the
container.

A one-off command - pipe it in, and the container is gone when it finishes:

```sh
$ bagworm enter <<< 'npm test'
```

Several commands work the same way with a heredoc:

```sh
$ bagworm enter <<'EOF'
npm ci
npm run build
npm test
EOF
```

A toolchain you never installed. Given this `bagworm.yaml`:

```yaml
image: golang:1.25
```

you can hack on a Go project on a machine that has no Go at all:

```sh
$ bagworm enter <<< 'go test ./...'
ok      example.com/yourproject/internal/thing   0.31s
```

CI is the same line. Bagworm only adds `-t` when stdout is a terminal, so
there is nothing to configure:

```yaml
# .github/workflows/test.yml, the relevant step
- run: bagworm enter <<< 'make test'
```

## What Bagworm does for you

- **Runtime detection** - podman -> docker -> nerdctl, first one installed
  wins. Nothing in the config, CLI, or docs requires you to know which.
- **User mapping** - rootless podman gets `--userns=keep-id`; docker/nerdctl
  get `--user <uid>:<gid>` with a writable `HOME`. Either way, files land
  owned by you.
- **Identity mounts** - `~/.gitconfig`, `~/.ssh`, and `~/.config/gh` are
  mounted read-only into the container, each only if it exists on your host.
- **TTY handling** - `-t` only when stdout is a terminal, so
  `bagworm enter <<< 'make test'` and CI pipelines just work.
- **SELinux** - detected automatically; workspace mounts get `:z` on
  enforcing hosts (identity mounts are never relabeled - you get a warning
  instead).
- **Clean handoff** - bagworm `exec`s into the runtime and ceases to exist,
  so signals, TTY resize, and exit codes come straight from the runtime.

Curious what it would run? `bagworm enter --dry-run` prints the exact
command, shell-quoted, one argument per line.

## What Bagworm will never do

1. **Expose raw runtime flags.** The moment `--docker-args` exists, Bagworm
   is just another wrapper, so it will never have that escape hatch.
2. **Build images.** Bagworm inhabits, it does not construct.
3. **Grow features that don't remove a daily annoyance.** "Docker can do X"
   is not a reason for Bagworm to do X. The config schema *is* the feature
   set.

## Install

From source (Go 1.25+):

```sh
go install github.com/Smiduweorc/bagworm/cmd/bagworm@latest
```

From a release binary:

```sh
curl -sSfL https://raw.githubusercontent.com/Smiduweorc/bagworm/master/install.sh | sh
```

Arch Linux: build with the included [`PKGBUILD`](./PKGBUILD)
(`makepkg -si`).

You'll also need at least one of podman, docker, or nerdctl.

## Known quirks (v0.1)

- On docker/nerdctl your user doesn't exist in the image's `/etc/passwd`,
  so the prompt says `I have no name!`. Cosmetic; your files are still
  yours. Podman users don't see this.
- Images with *no* shell at all fail with the runtime's own error message.
  Acceptable: bagworm exists to put you in a shell.
- Rootful podman is untested; v0.1 assumes rootless.

## Development

```sh
go test ./...                             # unit tests, hermetic, no containers
go test -tags integration ./internal/enter/  # needs a real runtime
```

Hooks (gofmt/vet/test + Conventional Commits) install via `npm install`
(lefthook). Changelog is generated with `npm run changelog` (git-cliff).

## License

MIT - see [LICENSE](./LICENSE).

---

Attribution for the metal can in the logo:
<a href="https://www.vecteezy.com/free-png/aluminum-can">Aluminum Can PNGs by Vecteezy</a>
