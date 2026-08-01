// Command fixture serves a small site that exercises every tunnel feature:
// plain fetch, redirect following, large download, chunked upload, cookie
// mirroring, and a WebSocket echo. The page renders "ALL CHECKS PASSED" into
// #result once every probe succeeds, which the Playwright e2e test asserts on
// through a real PunchPage tunnel.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

const largeSize = 300000

const page = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>PunchPage E2E fixture</title></head>
<body>
<h1>PunchPage E2E fixture</h1>
<p id="result">Testing…</p>
<ul id="checks"></ul>
<script>
(() => {
  const names = ['page', 'api', 'redirect', 'large', 'upload', 'cookie', 'websocket'];
  const state = {page: 'ok'};
  const render = () => {
    document.querySelector('#checks').innerHTML =
      names.map(name => '<li>' + name + ': ' + (state[name] || 'pending') + '</li>').join('');
    if (names.every(name => state[name] === 'ok')) {
      document.querySelector('#result').textContent = 'ALL CHECKS PASSED';
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

func main() {
	port := flag.Int("port", 8213, "listen port")
	flag.Parse()

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

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	log.Printf("fixture listening on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
