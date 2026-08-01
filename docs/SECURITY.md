# Security

## Model

- **The link is the password.** Anyone holding the share URL can browse the shared origin while the host runs. Share it like a credential; stop the host to revoke.
- **Secrets never reach a server.** The room id and encryption key travel in the URL fragment, which browsers do not send to GitHub Pages.
- **Signaling is encrypted.** Offer/answer/ICE messages are AES-256-GCM encrypted before hitting public Nostr relays; relays see only ciphertext and a random room tag.
- **Traffic is peer-to-peer.** Site bytes flow over a DTLS-encrypted WebRTC data channel directly between host and viewer. Neither Pages nor relays can observe or modify them.

## Limitations

- No TURN fallback by design (nothing in the middle to run or trust) — networks that block peer-to-peer WebRTC (symmetric NAT, strict corporate networks, some CGNAT) fail rather than degrade.
- Public relays and STUN servers (Google, Cloudflare) are best-effort third parties; they can observe connection metadata (IPs, timing) but not content.
- The host forwards requests to one local origin only; it is not a general proxy into your machine.

## Reporting

Found a vulnerability? Open a GitHub security advisory or contact the maintainer privately rather than filing a public issue.
