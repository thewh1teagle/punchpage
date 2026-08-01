<div align="center">

<a href="https://punchpage.pages.dev">
  <img src=".github/assets/logo.svg" width="76" alt="PunchPage">
</a>

<h1>PunchPage</h1>

<p>
  <b>Like ngrok, but peer-to-peer.</b><br>
  Share a local web app through a plain browser URL. Traffic goes straight between the two machines, end-to-end encrypted, with nothing to sign up for.
</p>

<p>
  <a href="https://punchpage.pages.dev"><img alt="Website" src="https://img.shields.io/badge/website-punchpage.pages.dev-6d28d9?style=flat-square"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-6d28d9?style=flat-square"></a>
  <a href="https://ko-fi.com/thewh1teagle"><img alt="Support on Ko-fi" src="https://img.shields.io/badge/Ko--fi-support-ff5e5b?style=flat-square&logo=ko-fi&logoColor=white"></a>
</p>

<img src=".github/assets/hero.gif" width="820" alt="PunchPage demo">

</div>

## Install

macOS / Linux:

```sh
curl -fsSL https://punchpage.pages.dev/install.sh | sh
```

Windows (PowerShell):

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://punchpage.pages.dev/install.ps1 | iex"
```

## Use

```console
$ punch 3000
PunchPage is sharing http://127.0.0.1:3000

  https://punchpage.pages.dev/#r=D3q9ZlzGGR9KAZ&k=HM5qyJ8Ezcol0Q1Yh
```

Takes a port, a host:port, a full URL, or `demo`. Send the printed link to anyone. Uploads, cookies, SSE and WebSockets all work. See `punch -h` for flags.

Using an AI assistant? Paste this:

```text
Share my local app on port 3000 with PunchPage, then give me the link.
Instructions: https://punchpage.pages.dev/llms.txt
```

## Scope

Good for handing your localhost to a person: demos, design reviews, your app on your phone. The link dies with the process.

Not for being publicly reachable. No server in the middle means no public endpoint, so webhooks, OAuth callbacks and hosting need ngrok or cloudflared instead.

## Docs

- [Architecture](docs/ARCHITECTURE.md): how the tunnel works
- [Security](docs/SECURITY.md): threat model and limitations
- [Building](docs/BUILDING.md): build and test from source
- [Deployment](docs/DEPLOYMENT.md): hosting, secrets, and releases

## License

MIT
