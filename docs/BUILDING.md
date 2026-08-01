# Building

## Prerequisites

- Go 1.25+
- Node 24 + [pnpm](https://pnpm.io) (browser client, e2e tests)
- [just](https://github.com/casey/just) (task runner, optional but recommended)

## Tasks

Run `just` to list all recipes (build, lint, test, e2e, demo, …). Without `just`, the recipes are one-liners; see the `justfile`.

## Layout

Go host in `cmd/punch` + `internal/` (the binary is `punch`), TypeScript client in `web/` (Vite, strict TS), e2e suite in `e2e/`. The client deploys to Cloudflare Pages (and a GitHub Pages mirror) automatically on pushes touching `web/`.

## Releasing

Tag and push: `git tag vX.Y.Z && git push --tags`. GoReleaser builds binaries for macOS/Linux/Windows (amd64 + arm64) and publishes a GitHub Release with archives named `punch_<os>_<arch>` that the install scripts in `scripts/` download.
