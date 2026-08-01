# PunchPage

Share a local web app through an ordinary browser URL with direct peer-to-peer
application traffic and no account, VPS, router configuration, visitor app,
extension, VPN, or payload relay.

```console
$ punchpage --target http://127.0.0.1:3000

PunchPage is sharing http://127.0.0.1:3000

  https://thewh1teagle.github.io/punchpage/#r=...&k=...&relays=...
```

Send that capability URL to a friend. Chrome or Brave loads the GitHub Pages
client, exchanges encrypted WebRTC signaling through several public Nostr
relays, and then requests the local site over a direct WebRTC data channel.

## Why there is no hosted PunchPage backend

- GitHub Pages serves the static browser client.
- Public Nostr relays carry short-lived, AES-256-GCM-encrypted signaling events.
- Google and Cloudflare public STUN services discover direct ICE candidates.
- The local site's HTML, JavaScript, API, upload, download, and WebSocket bytes
  travel only between the browser and the host.

The key is stored in the URL fragment. Browsers never include a fragment in the
HTTP request to GitHub Pages, and the relays only receive ciphertext. PunchPage
uses Nostr's ephemeral event range, so conforming relays do not persist the
events.

## Build and use

Go 1.25 or newer is currently used by the project:

```sh
go build -o punchpage ./cmd/punchpage
./punchpage --target http://127.0.0.1:3000
```

Useful options:

```text
--target       local HTTP origin (default http://127.0.0.1:3000)
--interface    optional interface restriction, such as en0
--page         alternate deployed browser-client URL
--relays       comma-separated wss:// Nostr relay URLs
--room, --key  resume a specific capability instead of generating one
```

The default public client is configured for this repository:
`https://thewh1teagle.github.io/punchpage/`.

## Supported browser traffic

The bridge multiplexes and streams concurrent HTTP requests and supports request
bodies, downloads, redirects, ranges, headers, cookies, client-side fetch/XHR,
and WebSockets. It injects a small compatibility runtime before application
scripts, which also allows Vite hot-module reload to work.

The end-to-end validation app passes React/Vite module loading, CSS, JSON APIs,
redirects, a 300 KB response, a 100 KB upload, cookies, Vite HMR, and an
application WebSocket.

## Boundaries

- Direct WebRTC must be possible. PunchPage deliberately has no TURN or payload
  fallback, so networks that block peer-to-peer WebRTC will fail rather than
  relay the website.
- The anonymous public Nostr relays are redundant but best-effort and may change
  their access policies.
- DRM, raw WebTransport, browser extensions, and other APIs outside HTTP and
  WebSocket are not emulated.
- A GitHub project Pages URL has a `/punchpage/` scope. Most React/Vite apps work
  through PunchPage's path rewriting; applications that inspect the browser's
  raw pathname may prefer a custom root domain.
- Anyone holding the capability URL can access the shared origin while the host
  process is running. Treat it as a password.

## Browser client development

The source is in `web/`; its production output is generated in `docs/`:

```sh
cd web
pnpm install
pnpm build
```

The Pages workflow rebuilds and deploys `docs/` on every push to `main`.

## License

MIT
