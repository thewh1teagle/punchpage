// Package demo serves the built-in demo site: a page that probes every
// tunnel feature (fetch, redirects, large downloads, uploads, cookies, and
// WebSockets) and reports "ALL CHECKS PASSED" once each probe succeeds. The
// binary embeds it for `punchpage demo`, and the e2e suite serves it as the
// test fixture.
package demo

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

const largeSize = 300000

const page = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>PunchPage demo</title>
<style>
:root{
  color-scheme:light dark;
  --bg:#f4f4f6;--glow:rgba(99,102,241,.07);
  --panel:rgba(255,255,255,.82);--line:rgba(17,19,27,.08);
  --text:#16181f;--muted:#5d6370;--faint:#868c99;
  --accent:#4f46e5;--accent-soft:rgba(79,70,229,.16);
  --bad:#b3261e;
  --shadow:0 1px 1px rgba(17,19,27,.03),0 4px 10px rgba(17,19,27,.04),0 18px 44px rgba(17,19,27,.08);
}
@media (prefers-color-scheme:dark){:root{
  --bg:#101116;--glow:rgba(129,140,248,.08);
  --panel:rgba(24,26,33,.86);--line:rgba(255,255,255,.09);
  --text:#eceef2;--muted:#a3a9b7;--faint:#767d8c;
  --accent:#a5b0fb;--accent-soft:rgba(129,140,248,.18);
  --bad:#f2a8a3;
  --shadow:0 1px 1px rgba(0,0,0,.25),0 6px 16px rgba(0,0,0,.3),0 24px 60px rgba(0,0,0,.42);
}}
body{
  margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;
  background:radial-gradient(52rem 34rem at 50% -12%,var(--glow),transparent 68%),var(--bg);
  color:var(--text);
  font-family:system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif;
  -webkit-font-smoothing:antialiased;
}
main{
  position:relative;max-width:400px;width:90%;box-sizing:border-box;padding:36px 32px 26px;
  border:1px solid var(--line);border-radius:18px;background:var(--panel);
  box-shadow:var(--shadow);backdrop-filter:blur(14px);-webkit-backdrop-filter:blur(14px);
  overflow:hidden;animation:card-in .5s cubic-bezier(.22,1,.36,1) both;
}
main::before{
  content:"";position:absolute;inset:0 0 auto;height:1px;
  background:linear-gradient(90deg,transparent 8%,var(--accent-soft) 50%,transparent 92%);
}
@keyframes card-in{from{opacity:0;transform:translateY(10px) scale(.985)}to{opacity:1;transform:none}}
h1{margin:0 0 6px;font-size:17px;font-weight:600;letter-spacing:-.015em}
h1 b{font-weight:600;color:var(--accent)}
p.sub{margin:0 0 22px;color:var(--muted);font-size:13.5px;line-height:1.6}
#result{
  margin:0 0 14px;font-size:13px;font-weight:600;letter-spacing:.09em;
  text-transform:uppercase;color:var(--muted);transition:color .3s ease;
}
#result.pass{color:var(--accent);animation:pass-in .45s cubic-bezier(.22,1,.36,1) both}
@keyframes pass-in{from{opacity:0;transform:translateY(4px)}to{opacity:1;transform:none}}
ul{list-style:none;margin:0;padding:0;border-top:1px solid var(--line)}
li{
  display:flex;align-items:center;justify-content:space-between;gap:12px;
  padding:10px 2px;border-bottom:1px solid var(--line);font-size:13.5px;
}
li .name{color:var(--text);letter-spacing:.01em}
li .state{display:inline-flex;align-items:center;gap:6px;font-size:12.5px;font-weight:500}
li .state svg{width:14px;height:14px;flex:none}
li .ok{color:var(--accent);animation:state-in .35s cubic-bezier(.22,1,.36,1) both}
li .bad{color:var(--bad);animation:state-in .35s cubic-bezier(.22,1,.36,1) both}
li .pending{color:var(--faint)}
li .pending .dot{
  width:6px;height:6px;border-radius:50%;background:var(--faint);
  animation:pulse 1.4s ease-in-out infinite;
}
@keyframes state-in{from{opacity:0;transform:translateX(4px)}to{opacity:1;transform:none}}
@keyframes pulse{0%,100%{opacity:.35}50%{opacity:1}}
@media (prefers-reduced-motion:reduce){
  main,#result.pass,li .ok,li .bad{animation:none}
  li .pending .dot{animation:none}
}
footer{margin-top:20px;padding-top:16px;border-top:1px solid var(--line);color:var(--faint);font-size:12px;letter-spacing:.01em}
</style>
</head>
<body>
<main>
<h1>Punch<b>Page</b> demo</h1>
<p class="sub">This page traveled peer-to-peer from the host&rsquo;s machine &mdash; no server in between.</p>
<p id="result">Testing&hellip;</p>
<ul id="checks"></ul>
<footer>Share this link with a friend &mdash; it works from anywhere.</footer>
</main>
<script>
(() => {
  const names = ['page', 'api', 'redirect', 'large', 'upload', 'cookie', 'websocket'];
  const state = {page: 'ok'};
  const checkIcon = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 8.5l3.2 3.2L13 4.5"/></svg>';
  const crossIcon = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><path d="M4.5 4.5l7 7M11.5 4.5l-7 7"/></svg>';
  const badges = {
    ok: checkIcon + 'Passed',
    bad: crossIcon + 'Failed'
  };
  const render = () => {
    document.querySelector('#checks').innerHTML = names
      .map(name => '<li><span class="name">' + name + '</span><span class="state ' + (state[name] || 'pending') + '">' +
        (badges[state[name]] || '<span class="dot"></span>Pending') + '</span></li>')
      .join('');
    if (names.every(name => state[name] === 'ok')) {
      const result = document.querySelector('#result');
      result.textContent = 'ALL CHECKS PASSED';
      result.className = 'pass';
    }
  };
  const update = (name, value) => { state[name] = value; render(); };
  render();
  (async () => {
    update('api', (await (await fetch('/api/status')).json()).api);
    update('redirect', (await (await fetch('/api/redirect')).json()).api);
    update('large', (await (await fetch('/api/large')).arrayBuffer()).byteLength === 300000 ? 'ok' : 'bad');
    const upload = new Uint8Array(100000).fill(7);
    update('upload', await (await fetch('/api/upload', {method: 'POST', body: upload})).text() === '100000' ? 'ok' : 'bad');
    await fetch('/api/cookie');
    update('cookie', document.cookie.includes('punchpage=direct') ? 'ok' : 'bad');
    const socket = new WebSocket('ws://' + location.host + '/socket');
    socket.onopen = () => socket.send('p2p-websocket');
    socket.onmessage = event => { update('websocket', event.data === 'p2p-websocket' ? 'ok' : 'bad'); socket.close(); };
    socket.onerror = () => update('websocket', 'bad');
  })().catch(error => update('error', error.message));
})();
</script>
</body>
</html>`

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// NewHandler returns the demo site handler.
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, page)
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"api":"ok"}`)
	})
	mux.HandleFunc("/api/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/status", http.StatusFound)
	})
	mux.HandleFunc("/api/large", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		io.WriteString(w, strings.Repeat("punchpage-", largeSize/10)[:largeSize])
	})
	mux.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, "%d", len(body))
	})
	mux.HandleFunc("/api/cookie", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "punchpage=direct; Path=/; SameSite=Lax")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"cookie":"set"}`)
	})
	mux.HandleFunc("/socket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			kind, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(kind, data); err != nil {
				return
			}
		}
	})
	return mux
}
