![Logo](./assets/logo.png)

# bagworm

Put a `bagworm.yaml` in your project and run `bagworm enter`. You get a
shell in the image you named, with the project mounted at `/workspace` and
files owned by you on the host.

## Install

### Arch Linux (AUR)

```sh
$ yay -S bagworm      # or: paru -S bagworm
```

Or with `makepkg`, from the [AUR](https://aur.archlinux.org/packages/bagworm)
or from this repo's [`PKGBUILD`](./PKGBUILD), which is the published one:

```sh
$ git clone https://aur.archlinux.org/bagworm.git
$ cd bagworm
$ makepkg -si
```

The package installs `/usr/bin/bagworm`, completions for bash, zsh and fish,
and the annotated config at `/usr/share/doc/bagworm/bagworm.example.yaml`.
The container runtimes are `optdepends`, so install one yourself.

### Other platforms

From source (Go 1.26+):

```sh
$ go install github.com/Smiduweorc/bagworm/cmd/bagworm@latest
```

From a release binary:

```sh
$ curl -sSfL https://raw.githubusercontent.com/Smiduweorc/bagworm/master/install.sh | sh
```

You need podman, docker or nerdctl.

## Quick start

Only `image` is required:

```yaml
# bagworm.yaml
image: node:22
```

Enter from anywhere in the project:

```sh
$ cd deep/inside/your/project
$ bagworm enter
node@workspace:/workspace$ 
```

Exit the shell and the container is gone. It always runs with `--rm`; state
lives in the mounts.

## Configuration

This is the whole schema:

```yaml
# bagworm.yaml
image: node:22          # required

mounts:                 # default: [".:/workspace"]
  - .:/workspace        # relative sources resolve against the project root
  - ~/data:/data        # ~ expands to your home; destinations must be absolute

workdir: /workspace     # default: /workspace

command:                # default: bash if the image has it, sh otherwise
  - bash

identity:               # default: none; host credentials to mount read-only
  - git                 # ~/.gitconfig
  - ssh                 # ~/.ssh
  - gh                  # ~/.config/gh (GitHub CLI tokens)
```

[`bagworm.example.yaml`](./bagworm.example.yaml) is the annotated version.

The project root is wherever `bagworm.yaml` is, found by walking upward like
git, so `bagworm enter` works from any subdirectory. An unknown key is an
error that names the key and line number.

### Trust

Because the config is found by walking upward, the one that applies may not
be yours: a cloned repo, a shared parent directory, `/tmp`. It picks the
image, the command and the mounts, so `bagworm enter` only runs a config you
have approved:

- The first time, and after any change to the file (one a `git pull`
  brought in, say), it shows the config and asks `Trust it and enter? [y/N]`.
- Without a terminal (CI) it refuses and tells you to run `bagworm trust`,
  which approves the config as it is now. `bagworm untrust` withdraws that.
- `bagworm enter --dry-run` works on an untrusted config and prints the
  exact runtime command, one argument per line.
- A config owned by another user, or writable by group or others, is
  refused, since it could change after you approved it.

Approvals live in `~/.config/bagworm/trusted.json` (or
`$XDG_CONFIG_HOME/bagworm/`), keyed by path and the SHA-256 of the contents.

### Identity

Git, ssh and GitHub CLI credentials are only mounted if the config lists them
under `identity`, and only if they exist on the host. They are read-only, but
read-only still means readable: anything running in the image can use your
ssh keys or `gh` token. `bagworm enter --no-identity` drops them all for one
run.

## Everyday use

The container shell reads stdin, so anything you would type you can pipe in.

```sh
$ bagworm enter
node@workspace:/workspace$ npm run build
node@workspace:/workspace$ exit
$ ls -l dist/app.js
-rw-r--r-- 1 you you 48210 Jul  4 14:12 dist/app.js
```

One command, or several:

```sh
$ bagworm enter <<< 'npm test'

$ bagworm enter <<'EOF'
npm ci
npm run build
npm test
EOF
```

With `image: golang:1.25` you can run a Go project's tests on a machine with
no Go installed:

```sh
$ bagworm enter <<< 'go test ./...'
ok      example.com/yourproject/internal/thing   0.31s
```

In CI, `bagworm trust` approves the config in the checkout being tested:

```yaml
# .github/workflows/test.yml
- run: bagworm trust && bagworm enter <<< 'make test'
```

## How it runs the container

- It uses the first runtime it finds: podman, then docker, then nerdctl.
- Rootless podman gets `--userns=keep-id`; docker and nerdctl get
  `--user <uid>:<gid>` and a writable `HOME`. Either way, files you create
  are owned by you.
- `-t` is added only when stdout is a terminal, so piped input and CI work
  without flags.
- On an SELinux-enforcing host, workspace mounts get `:z`. Identity mounts
  are never relabeled; you get a warning that SELinux may block them.
- bagworm `exec`s the runtime, so signals, terminal resize and exit codes
  come straight from it.

## Not included

- Raw runtime flags. There is no `--docker-args`; use docker directly.
- Building images. Use `docker build` or `podman build` and name the result
  in `image`.
- Anything that doesn't remove a daily annoyance. "Docker can do X" is not a
  reason for bagworm to do X.

## Known quirks (v0.1)

- On docker/nerdctl your user doesn't exist in the image's `/etc/passwd`,
  so the prompt says `I have no name!`. Cosmetic; your files are still
  yours. Podman users don't see this.
- Images with no shell at all fail with the runtime's own error message.
  Acceptable: bagworm exists to put you in a shell.
- Rootful podman is untested; v0.1 assumes rootless.

## License

MIT, see [LICENSE](./LICENSE).

Attribution for the metal can in the logo:
<a href="https://www.vecteezy.com/free-png/aluminum-can">Aluminum Can PNGs by Vecteezy</a>
