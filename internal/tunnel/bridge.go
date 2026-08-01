// Package tunnel forwards HTTP requests and WebSocket connections received
// over a WebRTC data channel to a local HTTP origin, rewriting responses so
// the site works when served from the GitHub Pages client's path prefix.
package tunnel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"

	"github.com/thewh1teagle/punchpage/internal/wire"
)

// chunkSize is the maximum body chunk (pre-base64) per data-channel frame.
const chunkSize = 32 << 10

// incomingRequest accumulates a request's metadata and body chunks until the
// browser signals "request-end".
type incomingRequest struct {
	message wire.Message
	body    bytes.Buffer
}

// dataSender serializes wire messages onto a WebRTC data channel, guarding
// against concurrent writes.
type dataSender struct {
	dc *webrtc.DataChannel
	mu sync.Mutex
}

func (s *dataSender) send(message wire.Message) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dc.SendText(string(data))
}

// wsPeer wraps an upstream WebSocket connection with a write lock.
type wsPeer struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *wsPeer) write(kind int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(kind, data)
}

// Bridge proxies one peer's traffic between a WebRTC data channel and the
// local HTTP origin. It owns an HTTP client with a cookie jar so upstream
// session cookies survive across requests, and tracks in-flight requests and
// open WebSockets by ID.
type Bridge struct {
	base       *url.URL
	client     *http.Client
	sender     *dataSender
	requestsMu sync.Mutex
	requests   map[string]*incomingRequest
	cancels    map[string]context.CancelFunc
	websocksMu sync.Mutex
	websocks   map[string]*wsPeer
}

// NewBridge creates a Bridge that forwards traffic from dc to the local
// origin at base. Redirects are passed through to the browser rather than
// followed.
func NewBridge(base *url.URL, dc *webrtc.DataChannel) *Bridge {
	jar, _ := cookiejar.New(nil)
	return &Bridge{
		base: base,
		client: &http.Client{
			Jar:     jar,
			Timeout: 0,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		sender:   &dataSender{dc: dc},
		requests: make(map[string]*incomingRequest),
		cancels:  make(map[string]context.CancelFunc),
		websocks: make(map[string]*wsPeer),
	}
}

// Handle dispatches one raw data-channel frame from the browser.
func (b *Bridge) Handle(data []byte) {
	var message wire.Message
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

// failRequest drops any buffered request state for id and reports an error to
// the browser.
func (b *Bridge) failRequest(id, reason string) {
	b.requestsMu.Lock()
	delete(b.requests, id)
	b.requestsMu.Unlock()
	_ = b.sender.send(wire.Message{Type: "response-error", ID: id, Error: reason})
}

// fetch performs the buffered request against the local origin and streams
// the response back over the data channel. Text assets (HTML, JS, CSS) are
// rewritten for the client's path prefix before sending.
func (b *Bridge) fetch(incoming *incomingRequest) {
	m := incoming.message
	relative, err := url.Parse(m.URL)
	if err != nil || relative.IsAbs() || !strings.HasPrefix(relative.Path, "/") {
		_ = b.sender.send(wire.Message{Type: "response-error", ID: m.ID, Error: "invalid request URL"})
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
		_ = b.sender.send(wire.Message{Type: "response-error", ID: m.ID, Error: err.Error()})
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
			_ = b.sender.send(wire.Message{Type: "response-error", ID: m.ID, Error: err.Error()})
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
			_ = b.sender.send(wire.Message{Type: "response-error", ID: m.ID, Error: readErr.Error()})
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

// sendResponse streams status, headers, cookies, and chunked body frames for
// one response, terminating with "response-end" or "response-error".
func (b *Bridge) sendResponse(id string, status int, headers http.Header, cookies []string, reader io.Reader) {
	if err := b.sender.send(wire.Message{Type: "response-start", ID: id, Status: status, Headers: headers, Cookies: cookies}); err != nil {
		return
	}
	buffer := make([]byte, chunkSize)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			if sendErr := b.sender.send(wire.Message{Type: "response-body", ID: id, Data: base64.StdEncoding.EncodeToString(buffer[:n])}); sendErr != nil {
				return
			}
		}
		if err == io.EOF {
			_ = b.sender.send(wire.Message{Type: "response-end", ID: id})
			return
		}
		if err != nil {
			_ = b.sender.send(wire.Message{Type: "response-error", ID: id, Error: err.Error()})
			return
		}
	}
}

// openWebSocket dials the requested path on the local origin (carrying the
// bridge's cookies) and relays upstream frames back to the browser until the
// connection closes.
func (b *Bridge) openWebSocket(message wire.Message) {
	requested, err := url.Parse(message.URL)
	if err != nil {
		_ = b.sender.send(wire.Message{Type: "ws-error", ID: message.ID, Error: "invalid WebSocket URL"})
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
		_ = b.sender.send(wire.Message{Type: "ws-error", ID: message.ID, Error: detail})
		return
	}
	peer := &wsPeer{conn: conn}
	b.websocksMu.Lock()
	b.websocks[message.ID] = peer
	b.websocksMu.Unlock()
	_ = b.sender.send(wire.Message{Type: "ws-opened", ID: message.ID, Protocol: conn.Subprotocol()})
	log.Printf("P2P WebSocket open %s", requested.RequestURI())
	for {
		kind, payload, readErr := conn.ReadMessage()
		if readErr != nil {
			code, reason := websocket.CloseNormalClosure, ""
			if closeErr, ok := readErr.(*websocket.CloseError); ok {
				code, reason = closeErr.Code, closeErr.Text
			}
			_ = b.sender.send(wire.Message{Type: "ws-close", ID: message.ID, Code: code, Reason: reason})
			break
		}
		_ = b.sender.send(wire.Message{Type: "ws-message", ID: message.ID, Binary: kind == websocket.BinaryMessage, Data: base64.StdEncoding.EncodeToString(payload)})
	}
	b.websocksMu.Lock()
	delete(b.websocks, message.ID)
	b.websocksMu.Unlock()
	_ = conn.Close()
}

// closeWebSocket sends a close frame upstream (defaulting to a normal
// closure) and forgets the connection.
func (b *Bridge) closeWebSocket(id string, code int, reason string) {
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
