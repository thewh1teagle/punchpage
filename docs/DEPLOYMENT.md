# Deployment

How PunchPage ships: one push deploys the browser client and the install scripts; one tag publishes binaries.

## Hosting

The client is a static site (`web/dist`), deployed to two places on every push touching `web/`:

- **[Cloudflare Pages](https://punchpage.pages.dev)** — the default. Served at the domain root, so the service worker's scope is `/` and no path-prefix rewriting is needed.
- **[GitHub Pages](https://thewh1teagle.github.io/punchpage/)** — kept live so older share links keep working.

The workflow copies `scripts/install.sh` and `scripts/install.ps1` into the site, so the installers live at `punchpage.pages.dev/install.sh` and `/install.ps1`.

## Setup (one time)

1. Create a Cloudflare Pages project named `punchpage`:
   ```sh
   pnpx wrangler pages project create punchpage --production-branch=main
   ```
2. Create an API token at [dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens) → Create Custom Token, with permission `Account · Cloudflare Pages · Edit`.
3. Add both repo secrets (values are read from stdin, never shell history):
   ```sh
   gh secret set CLOUDFLARE_API_TOKEN
   gh secret set CLOUDFLARE_ACCOUNT_ID   # pnpx wrangler whoami
   ```

The Cloudflare step is skipped automatically when the token secret is absent, so forks build fine without it.

## Releasing

```sh
git tag vX.Y.Z && git push --tags
```

[GoReleaser](https://goreleaser.com) builds macOS/Linux/Windows binaries (amd64 + arm64) and publishes a [GitHub Release](https://github.com/thewh1teagle/punchpage/releases). The install scripts always fetch `releases/latest`, so a new tag is all it takes to ship an update.

## Workflows

| Workflow | Trigger | Does |
| --- | --- | --- |
| `pages.yml` | push to `web/**` | typecheck, build, deploy to Cloudflare + GitHub Pages |
| `e2e.yml` | push / PR | full tunnel test through real relays and headless Chromium |
| `release.yml` | `v*` tag | GoReleaser build + GitHub Release |

## Rotating the token

Delete the old token in the Cloudflare dashboard, create a replacement with the same permission, and re-run `gh secret set CLOUDFLARE_API_TOKEN`. Rotate immediately if a token is ever pasted somewhere it shouldn't be.
