# Contributing to clipad

Issues and pull requests are welcome — bug reports, new AI shortcuts for the
default library, and terminal compatibility fixes especially.

## Prerequisites

- Go 1.26+

## Clone and build

```bash
git clone https://github.com/krzysztofciepka/clipad.git
cd clipad
go build -o clipad .
```

## Run the tests

```bash
go test ./...
```

## Layout

All source lives in a single root package; tests are table-driven
`*_test.go` next to the code they cover — follow that pattern. There is no
Makefile or code generation step: plain `go build` / `go test` is the whole
toolchain.

## Manual testing

clipad reads and writes its config under `~/.config/clipad` by default. To
try changes without touching your real vault or config, point `XDG_CONFIG_HOME`
at a scratch directory and run the binary against a throwaway vault:

```bash
mkdir -p /tmp/clipad-scratch/config /tmp/clipad-scratch/vault
XDG_CONFIG_HOME=/tmp/clipad-scratch/config ./clipad /tmp/clipad-scratch/vault
```

The first run prompts for the vault path and writes config to
`/tmp/clipad-scratch/config/clipad/config.toml`, leaving your real setup
untouched.

## Before opening a PR

- Keep PRs small and focused on one change.
- Include tests for new behavior or bug fixes.
- Make sure `go vet ./...` is clean.
- Make sure `go build ./...` and `go test ./...` pass.

See [README.md#quick-start](README.md#quick-start) for install and usage
basics.
