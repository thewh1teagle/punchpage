# Architecture

PunchPage tunnels a local HTTP origin to any browser with zero infrastructure of its own: GitHub Pages serves the static client, public Nostr relays pass a few KB of encrypted signaling, and all site traffic flows peer-to-peer over WebRTC.

```
  YOUR MACHINE                                          VIEWER'S MACHINE

┌───────────────┐                                     ┌──────────────────┐
│  local app    │                                     │  browser         │
│  :3000        │                                     │                  │
│      ▲        │                                     │  static client   │
│      │        │        WebRTC data channel          │  loaded once     │
│  punchpage    │◄═══════════════════════════════════►│  from GitHub     │
│  host (Go)    │      direct, peer-to-peer,          │  Pages           │
└───────┬───────┘      DTLS encrypted                 └────────┬─────────┘
        │                                                      │
        │             ~1 KB encrypted handshake                │
        └──────────────► Nostr relays ◄────────────────────────┘
                   (signaling only — site traffic
                    never touches any server)
```

## Flow

1. The host generates a room id and a 32-byte key, and prints a share URL. Both live in the URL **fragment**, which browsers never send to the server — GitHub Pages sees nothing.
2. Host and client exchange WebRTC offer/answer/ICE candidates as AES-256-GCM-encrypted ephemeral Nostr events (kind 24242) on public relays.
3. ICE (with public STUN) punches through NATs and opens a data channel directly between the two machines. Relays carry no site traffic.
4. In the browser, a service worker intercepts every request under its scope and forwards it over the data channel as a JSON wire message; the host fetches from the local origin and streams the response back in chunks.

## Components

- **Host** (`cmd/` + `internal/`) — Go CLI: encrypted Nostr signaling, WebRTC sessions (pion), and the bridge from the data channel to the local origin (HTTP with a cookie jar, WebSocket proxying, path-prefix rewriting of HTML/JS/CSS). Also embeds the demo site.
- **Client** (`web/`) — TypeScript browser app: connection UI, signaling, a service worker that proxies requests over the data channel, and a runtime shim injected into tunneled pages that rewrites `fetch`/XHR/`EventSource` URLs and bridges `WebSocket`.

## Path prefix

The client serves tunneled pages under `/punchpage/`, so absolute URLs in apps would escape the service-worker scope. The host rewrites HTML/JS/CSS on the way through, and the injected runtime patches the network APIs at runtime. Apps that inspect the raw pathname may still misbehave; frameworks with a root-path setting (e.g. Gradio's `root_path`) can side-step rewriting entirely.
