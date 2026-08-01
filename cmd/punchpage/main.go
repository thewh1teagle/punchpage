package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	ossignal "os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

const wireChunkSize = 32 << 10

type signal struct {
	Type      string                     `json:"type"`
	SDP       *webrtc.SessionDescription `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit   `json:"candidate,omitempty"`
}

type wireMessage struct {
	Type      string              `json:"type"`
	ID        string              `json:"id,omitempty"`
	URL       string              `json:"url,omitempty"`
	Method    string              `json:"method,omitempty"`
	Headers   map[string][]string `json:"headers,omitempty"`
	Data      string              `json:"data,omitempty"`
	Status    int                 `json:"status,omitempty"`
	Error     string              `json:"error,omitempty"`
	Binary    bool                `json:"binary,omitempty"`
	Code      int                 `json:"code,omitempty"`
	Reason    string              `json:"reason,omitempty"`
	Protocols []string            `json:"protocols,omitempty"`
	Protocol  string              `json:"protocol,omitempty"`
	Cookies   []string            `json:"cookies,omitempty"`
}

type incomingRequest struct {
	message wireMessage
	body    bytes.Buffer
}

type dataSender struct {
	dc *webrtc.DataChannel
	mu sync.Mutex
}

func (s *dataSender) send(message wireMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dc.SendText(string(data))
}

type wsPeer struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *wsPeer) write(kind int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(kind, data)
}

type bridge struct {
	base       *url.URL
	client     *http.Client
	sender     *dataSender
	requestsMu sync.Mutex
	requests   map[string]*incomingRequest
	cancels    map[string]context.CancelFunc
	websocksMu sync.Mutex
	websocks   map[string]*wsPeer
}

func newBridge(base *url.URL, sender *dataSender) *bridge {
	jar, _ := cookiejar.New(nil)
	return &bridge{
		base: base,
		client: &http.Client{
			Jar:     jar,
			Timeout: 0,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		sender:   sender,
		requests: make(map[string]*incomingRequest),
		cancels:  make(map[string]context.CancelFunc),
		websocks: make(map[string]*wsPeer),
	}
}

func (b *bridge) handle(data []byte) {
	var message wireMessage
	if err := json.Unmarshal(data, &message); err != nil {
		log.Printf("bad data-channel message: %v", err)
		return
	}
	switch message.Type {
	case "request":
		b.requestsMu.Lock()
		b.requests[message.ID] = &incomingRequest{message: message}
		b.requestsMu.Unlock()
	case "request-body":
		decoded, err := base64.StdEncoding.DecodeString(message.Data)
		if err != nil {
			b.failRequest(message.ID, "invalid request body encoding")
			return
		}
		b.requestsMu.Lock()
		if request := b.requests[message.ID]; request != nil {
			_, _ = request.body.Write(decoded)
		}
		b.requestsMu.Unlock()
	case "request-end":
		b.requestsMu.Lock()
		request := b.requests[message.ID]
		delete(b.requests, message.ID)
		b.requestsMu.Unlock()
		if request != nil {
			go b.fetch(request)
		}
	case "request-cancel":
		b.requestsMu.Lock()
		if cancel := b.cancels[message.ID]; cancel != nil {
			cancel()
		}
		b.requestsMu.Unlock()
	case "ws-open":
		go b.openWebSocket(message)
	case "ws-send":
		b.websocksMu.Lock()
		peer := b.websocks[message.ID]
		b.websocksMu.Unlock()
		if peer != nil {
			payload, err := base64.StdEncoding.DecodeString(message.Data)
			if err == nil {
				kind := websocket.TextMessage
				if message.Binary {
					kind = websocket.BinaryMessage
				}
				_ = peer.write(kind, payload)
			}
		}
	case "ws-close":
		b.closeWebSocket(message.ID, message.Code, message.Reason)
	}
}

func (b *bridge) failRequest(id, reason string) {
	b.requestsMu.Lock()
	delete(b.requests, id)
	b.requestsMu.Unlock()
	_ = b.sender.send(wireMessage{Type: "response-error", ID: id, Error: reason})
}

func (b *bridge) fetch(incoming *incomingRequest) {
	m := incoming.message
	relative, err := url.Parse(m.URL)
	if err != nil || relative.IsAbs() || !strings.HasPrefix(relative.Path, "/") {
		_ = b.sender.send(wireMessage{Type: "response-error", ID: m.ID, Error: "invalid request URL"})
		return
	}
	destination := b.base.ResolveReference(relative)
	ctx, cancel := context.WithCancel(context.Background())
	b.requestsMu.Lock()
	b.cancels[m.ID] = cancel
	b.requestsMu.Unlock()
	defer func() {
		cancel()
		b.requestsMu.Lock()
		delete(b.cancels, m.ID)
		b.requestsMu.Unlock()
	}()

	method := m.Method
	if method == "" {
		method = http.MethodGet
	}
	request, err := http.NewRequestWithContext(ctx, method, destination.String(), bytes.NewReader(incoming.body.Bytes()))
	if err != nil {
		_ = b.sender.send(wireMessage{Type: "response-error", ID: m.ID, Error: err.Error()})
		return
	}
	prefix := firstHeader(m.Headers, "X-PunchPage-Prefix")
	for name, values := range m.Headers {
		if isHopHeader(name) || strings.EqualFold(name, "host") || strings.EqualFold(name, "accept-encoding") || strings.EqualFold(name, "X-PunchPage-Prefix") {
			continue
		}
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := b.client.Do(request)
	if err != nil {
		if ctx.Err() == nil {
			_ = b.sender.send(wireMessage{Type: "response-error", ID: m.ID, Error: err.Error()})
		}
		return
	}
	defer response.Body.Close()

	headers := response.Header.Clone()
	cookies := headers.Values("Set-Cookie")
	headers.Del("Set-Cookie")
	for name := range headers {
		if isHopHeader(name) {
			headers.Del(name)
		}
	}
	if location := headers.Get("Location"); location != "" {
		location = rewriteLocation(b.base, location)
		if prefix != "" && strings.HasPrefix(location, "/") && !strings.HasPrefix(location, "//") {
			location = prefix + location
		}
		headers.Set("Location", location)
	}

	contentType := strings.ToLower(headers.Get("Content-Type"))
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "javascript") || strings.Contains(contentType, "text/css") {
		body, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			_ = b.sender.send(wireMessage{Type: "response-error", ID: m.ID, Error: readErr.Error()})
			return
		}
		if strings.Contains(contentType, "text/html") {
			body = prepareHTML(body, prefix)
		} else if strings.Contains(contentType, "javascript") {
			body = rewriteJavaScript(body, prefix)
		} else {
			body = rewriteCSS(body, prefix)
		}
		headers.Del("Content-Length")
		headers.Del("Content-Encoding")
		b.sendResponse(m.ID, response.StatusCode, headers, cookies, bytes.NewReader(body))
		log.Printf("P2P HTTP %s %s -> %d bytes=%d", method, relative.RequestURI(), response.StatusCode, len(body))
		return
	}
	b.sendResponse(m.ID, response.StatusCode, headers, cookies, response.Body)
	log.Printf("P2P HTTP %s %s -> %d streamed", method, relative.RequestURI(), response.StatusCode)
}

func (b *bridge) sendResponse(id string, status int, headers http.Header, cookies []string, reader io.Reader) {
	if err := b.sender.send(wireMessage{Type: "response-start", ID: id, Status: status, Headers: headers, Cookies: cookies}); err != nil {
		return
	}
	buffer := make([]byte, wireChunkSize)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			if sendErr := b.sender.send(wireMessage{Type: "response-body", ID: id, Data: base64.StdEncoding.EncodeToString(buffer[:n])}); sendErr != nil {
				return
			}
		}
		if err == io.EOF {
			_ = b.sender.send(wireMessage{Type: "response-end", ID: id})
			return
		}
		if err != nil {
			_ = b.sender.send(wireMessage{Type: "response-error", ID: id, Error: err.Error()})
			return
		}
	}
}

func (b *bridge) openWebSocket(message wireMessage) {
	requested, err := url.Parse(message.URL)
	if err != nil {
		_ = b.sender.send(wireMessage{Type: "ws-error", ID: message.ID, Error: "invalid WebSocket URL"})
		return
	}
	wsURL := *b.base
	if wsURL.Scheme == "https" {
		wsURL.Scheme = "wss"
	} else {
		wsURL.Scheme = "ws"
	}
	wsURL.Path, wsURL.RawQuery = requested.Path, requested.RawQuery
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second, Subprotocols: message.Protocols}
	headers := http.Header{"Origin": []string{b.base.Scheme + "://" + b.base.Host}}
	for _, cookie := range b.client.Jar.Cookies(b.base) {
		headers.Add("Cookie", cookie.Name+"="+cookie.Value)
	}
	conn, response, err := dialer.DialContext(context.Background(), wsURL.String(), headers)
	if err != nil {
		detail := err.Error()
		if response != nil {
			detail = fmt.Sprintf("%s (HTTP %d)", detail, response.StatusCode)
		}
		_ = b.sender.send(wireMessage{Type: "ws-error", ID: message.ID, Error: detail})
		return
	}
	peer := &wsPeer{conn: conn}
	b.websocksMu.Lock()
	b.websocks[message.ID] = peer
	b.websocksMu.Unlock()
	_ = b.sender.send(wireMessage{Type: "ws-opened", ID: message.ID, Protocol: conn.Subprotocol()})
	log.Printf("P2P WebSocket open %s", requested.RequestURI())
	for {
		kind, payload, readErr := conn.ReadMessage()
		if readErr != nil {
			code, reason := websocket.CloseNormalClosure, ""
			if closeErr, ok := readErr.(*websocket.CloseError); ok {
				code, reason = closeErr.Code, closeErr.Text
			}
			_ = b.sender.send(wireMessage{Type: "ws-close", ID: message.ID, Code: code, Reason: reason})
			break
		}
		_ = b.sender.send(wireMessage{Type: "ws-message", ID: message.ID, Binary: kind == websocket.BinaryMessage, Data: base64.StdEncoding.EncodeToString(payload)})
	}
	b.websocksMu.Lock()
	delete(b.websocks, message.ID)
	b.websocksMu.Unlock()
	_ = conn.Close()
}

func (b *bridge) closeWebSocket(id string, code int, reason string) {
	b.websocksMu.Lock()
	peer := b.websocks[id]
	delete(b.websocks, id)
	b.websocksMu.Unlock()
	if peer == nil {
		return
	}
	if code == 0 {
		code = websocket.CloseNormalClosure
	}
	_ = peer.write(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason))
	_ = peer.conn.Close()
}

var (
	localOrigin  = regexp.MustCompile(`https?://(?:localhost|127\.0\.0\.1)(?::\d+)?`)
	rootHTMLPath = regexp.MustCompile(`(?i)(\b(?:src|href|action)=["'])/([^/])`)
)

func prepareHTML(body []byte, prefix string) []byte {
	text := localOrigin.ReplaceAllString(string(body), "")
	if prefix != "" {
		text = rootHTMLPath.ReplaceAllString(text, `${1}`+prefix+`/${2}`)
		text = string(rewriteJavaScript([]byte(text), prefix))
	}
	scope := prefix
	if index := strings.Index(scope, "__punchpage__/"); index >= 0 {
		scope = scope[:index]
	}
	prefixJSON, _ := json.Marshal(prefix)
	runtimeTag := `<script>window.__PUNCHPAGE_PREFIX__=` + string(prefixJSON) + `</script><script src="` + scope + `__punchpage_runtime__.js"></script>`
	lower := strings.ToLower(text)
	if index := strings.Index(lower, "<head"); index >= 0 {
		if end := strings.Index(text[index:], ">"); end >= 0 {
			position := index + end + 1
			return []byte(text[:position] + runtimeTag + text[position:])
		}
	}
	return []byte(runtimeTag + text)
}

func rewriteJavaScript(body []byte, prefix string) []byte {
	if prefix == "" {
		return body
	}
	text := string(body)
	for _, marker := range []string{`from "`, `from '`, `import "`, `import '`, `import("`, `import('`} {
		text = strings.ReplaceAll(text, marker+`/`, marker+prefix+`/`)
	}
	return []byte(text)
}

func rewriteCSS(body []byte, prefix string) []byte {
	if prefix == "" {
		return body
	}
	text := strings.ReplaceAll(string(body), "url(/", "url("+prefix+"/")
	text = strings.ReplaceAll(text, `url("/`, `url("`+prefix+`/`)
	text = strings.ReplaceAll(text, `url('/`, `url('`+prefix+`/`)
	return []byte(text)
}

func firstHeader(headers map[string][]string, wanted string) string {
	for name, values := range headers {
		if strings.EqualFold(name, wanted) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func rewriteLocation(base *url.URL, location string) string {
	parsed, err := url.Parse(location)
	if err != nil || !parsed.IsAbs() {
		return location
	}
	if parsed.Host == base.Host {
		return parsed.RequestURI()
	}
	return location
}

func isHopHeader(name string) bool {
	switch strings.ToLower(name) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func main() {
	page := flag.String("page", "https://thewh1teagle.github.io/punchpage/", "GitHub Pages browser URL")
	relayList := flag.String("relays", "wss://relay.damus.io,wss://nos.lol,wss://relay.primal.net", "comma-separated public Nostr relays")
	room := flag.String("room", "", "existing room identifier (normally generated)")
	keyText := flag.String("key", "", "existing base64url signaling key (normally generated)")
	target := flag.String("target", "http://127.0.0.1:3000", "local HTTP origin")
	iface := flag.String("interface", "", "optional network interface to expose to ICE")
	flag.Parse()

	base, err := url.Parse(*target)
	if err != nil {
		log.Fatal(err)
	}
	relays := splitRelays(*relayList)
	if len(relays) == 0 {
		log.Fatal("at least one --relays URL is required")
	}
	if *room == "" {
		*room = randomToken(16)
	}
	var key []byte
	if *keyText == "" {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			log.Fatal(err)
		}
		*keyText = base64.RawURLEncoding.EncodeToString(key)
	} else {
		key, err = base64.RawURLEncoding.DecodeString(*keyText)
		if err != nil || len(key) != 32 {
			log.Fatal("--key must be a 32-byte base64url value")
		}
	}
	shareURL, err := makeShareURL(*page, *room, *keyText, relays)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := ossignal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	signaler, err := newNostrSignaler(ctx, relays, *room, key)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\nPunchPage is sharing %s\n\n  %s\n\n", base, shareURL)
	log.Printf("encrypted signaling room=%s relays=%d", *room, len(relays))
	if err := runHost(ctx, signaler, *iface, base); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}

type peerSession struct {
	pc        *webrtc.PeerConnection
	remoteSet bool
	queued    []webrtc.ICECandidateInit
}

func runHost(ctx context.Context, signaler *nostrSignaler, iface string, base *url.URL) error {
	settings := webrtc.SettingEngine{}
	settings.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	if iface != "" {
		settings.SetInterfaceFilter(func(name string) bool { return name == iface })
	}
	api := webrtc.NewAPI(webrtc.WithSettingEngine(settings))
	configuration := webrtc.Configuration{ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.cloudflare.com:3478", "stun:stun.l.google.com:19302"}}}}
	sessions := make(map[string]*peerSession)
	defer func() {
		for _, session := range sessions {
			if session.pc != nil {
				_ = session.pc.Close()
			}
		}
	}()
	for {
		var incoming receivedSignal
		select {
		case incoming = <-signaler.messages:
		case <-ctx.Done():
			return ctx.Err()
		}
		message, peer := incoming.Signal, incoming.Peer
		session := sessions[peer]
		switch message.Type {
		case "offer":
			if message.SDP == nil {
				continue
			}
			queued := []webrtc.ICECandidateInit(nil)
			if session != nil {
				queued = session.queued
				if session.pc != nil {
					_ = session.pc.Close()
				}
			}
			pc, createErr := newPeerConnection(api, configuration, signaler, peer, base)
			if createErr != nil {
				log.Printf("peer=%s create: %v", peer, createErr)
				continue
			}
			session = &peerSession{pc: pc, queued: queued}
			sessions[peer] = session
			if err := pc.SetRemoteDescription(*message.SDP); err != nil {
				log.Printf("peer=%s remote description: %v", peer, err)
				continue
			}
			session.remoteSet = true
			for _, candidate := range session.queued {
				_ = pc.AddICECandidate(candidate)
			}
			session.queued = nil
			answer, err := pc.CreateAnswer(nil)
			if err != nil {
				continue
			}
			if err := pc.SetLocalDescription(answer); err != nil {
				continue
			}
			signaler.sendAsync(peer, signal{Type: "answer", SDP: pc.LocalDescription()})
		case "candidate":
			if message.Candidate == nil {
				continue
			}
			if session == nil {
				session = &peerSession{}
				sessions[peer] = session
			}
			if !session.remoteSet || session.pc == nil {
				session.queued = append(session.queued, *message.Candidate)
			} else {
				_ = session.pc.AddICECandidate(*message.Candidate)
			}
		}
	}
}

func newPeerConnection(api *webrtc.API, configuration webrtc.Configuration, signaler *nostrSignaler, peer string, base *url.URL) (*webrtc.PeerConnection, error) {
	pc, err := api.NewPeerConnection(configuration)
	if err != nil {
		return nil, err
	}
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			init := candidate.ToJSON()
			signaler.sendAsync(peer, signal{Type: "candidate", Candidate: &init})
		}
	})
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("peer=%s ICE state=%s", peer, state)
		if state == webrtc.ICEConnectionStateConnected || state == webrtc.ICEConnectionStateCompleted {
			pair, pairErr := pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
			if pairErr == nil {
				log.Printf("peer=%s DIRECT selected pair local=%s:%d(%s) remote=%s:%d(%s)", peer, pair.Local.Address, pair.Local.Port, pair.Local.Typ, pair.Remote.Address, pair.Remote.Port, pair.Remote.Typ)
			}
		}
	})
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		log.Printf("peer=%s data channel label=%s", peer, dc.Label())
		bridge := newBridge(base, &dataSender{dc: dc})
		dc.OnOpen(func() { log.Printf("peer=%s data channel open", peer) })
		dc.OnMessage(func(message webrtc.DataChannelMessage) { bridge.handle(message.Data) })
	})
	return pc, nil
}

func splitRelays(value string) []string {
	var relays []string
	for _, relay := range strings.Split(value, ",") {
		if relay = strings.TrimSpace(relay); strings.HasPrefix(relay, "wss://") {
			relays = append(relays, relay)
		}
	}
	return relays
}

func randomToken(size int) string {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func makeShareURL(page, room, key string, relays []string) (string, error) {
	parsed, err := url.Parse(page)
	localHTTP := parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1")
	if err != nil || (parsed.Scheme != "https" && !localHTTP) || parsed.Host == "" {
		return "", fmt.Errorf("--page must be an absolute HTTPS URL (or localhost for development)")
	}
	parsed.Fragment = "r=" + room + "&k=" + key + "&relays=" + strings.Join(relays, ",")
	return parsed.String(), nil
}

func init() { log.SetFlags(log.LstdFlags | log.Lmicroseconds) }
