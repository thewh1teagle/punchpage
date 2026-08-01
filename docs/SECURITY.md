# Security

## Model

- **The link is the password.** Anyone holding the share URL can browse the shared origin while the host runs. Share it like a credential; stop the host to revoke.
- **Secrets never reach a server.** The room id and encryption key travel in the URL fragment, which browsers do not send to the Pages host.
- **Signaling is encrypted.** Offer/answer/ICE messages are AES-256-GCM encrypted before hitting public Nostr relays; relays see only ciphertext and a random room tag.
- **Traffic is peer-to-peer.** Site bytes flow over a DTLS-encrypted WebRTC data channel directly between host and viewer. Neither Pages nor relays can observe or modify them.

## Limitations

- No TURN fallback by design (nothing in the middle to run or trust), so networks that block peer-to-peer WebRTC (symmetric NAT, strict corporate networks, some CGNAT) fail rather than degrade.
- Public relays and STUN servers (Google, Cloudflare) are best-effort third parties; they can observe connection metadata (IPs, timing) but not content.
- The host forwards requests to one local origin only; it is not a general proxy into your machine.

## Browser-side model

The viewer's browser runs the shared site inside the PunchPage client, on the client's own origin. Two consequences are worth understanding before sharing something you do not trust:

- **The shared site is same-origin with the client.** Its JavaScript can read the page URL, and therefore the session key, and can reach storage on the client origin. If the app you share is compromised (for example via XSS), the attacker can lift the key and join the tunnel independently from anywhere for as long as the host runs. Share apps you trust, and stop the host to revoke.
- **Sessions share one browser origin.** Cookies the host sets are mirrored into that origin, and site storage is not partitioned per session, so a later shared site can read what an earlier one left behind. The service worker also stays registered after the session ends. Use a private window for sensitive sessions if this matters to you.

Cookies work through a jar kept by the host, not by the browser. Each viewer gets their own bridge and their own jar, so logging in through the tunnel does not hand your session to anyone else holding the link; they reach the login page with an empty jar. Two caveats remain: cookies written by page JavaScript are not sent back to the host, and the jar is discarded when the connection drops, so a reconnect starts logged out.

## Reporting

Found a vulnerability? Open a GitHub security advisory or contact the maintainer privately rather than filing a public issue.
