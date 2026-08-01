# PunchPage

**Like ngrok, but peer-to-peer.**

Share a local web app through a plain browser URL — no server, account, or router configuration.

```console
$ punchpage --target http://127.0.0.1:3000

PunchPage is sharing http://127.0.0.1:3000

  https://thewh1teagle.github.io/punchpage/#r=...&k=...&relays=...
```

Send that URL to a friend. Their browser loads a static client from GitHub Pages and connects straight to your machine over WebRTC — requests, uploads, cookies, WebSockets, and Vite HMR all work.

Nothing in the middle sees your site: the room id and key live in the URL fragment (never sent to GitHub Pages), signaling is AES-256-GCM encrypted through public Nostr relays, and site bytes flow only between the two peers.

## Install and use

Requires Go 1.25+:

```sh
go build -o punchpage ./cmd/punchpage
./punchpage --target http://127.0.0.1:3000
```

Flags:

```text
--target      local HTTP origin to share (default http://127.0.0.1:3000)
--interface   optional network interface to expose to ICE (e.g. en0)
--page        browser client URL (default https://thewh1teagle.github.io/punchpage/)
--relays      comma-separated wss:// Nostr relays
--room, --key resume an existing share URL instead of generating a new one
```

The browser client source is in `web/` (`pnpm install && pnpm build`); the Pages workflow builds and deploys it automatically.

## Testing

`go test ./...` covers the host. End-to-end tests drive a real tunnel — fixture site → host → public Nostr relays → headless Chromium — and assert fetch, redirects, large downloads, uploads, cookies, and WebSockets all work:

```sh
just e2e   # or: cd e2e && pnpm install && pnpm e2e
```

A `justfile` collects the common tasks: `just build`, `just test`, `just lint`, `just web`, `just e2e`, `just run`.

## Security and limitations

- Anyone with the URL can reach the shared site while the host runs. Treat it as a password.
- No TURN or relay fallback: networks that block peer-to-peer WebRTC fail rather than degrade.
- Public Nostr relays and STUN servers (Google, Cloudflare) are best-effort third parties.
- The client is served under `/punchpage/`; path rewriting handles most apps, but apps that inspect the raw pathname may misbehave.

## License

MIT
