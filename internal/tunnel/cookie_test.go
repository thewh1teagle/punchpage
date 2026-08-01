package tunnel

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// Each viewer gets its own bridge (host.go builds one per data channel), so a
// login performed by one viewer must not leak into another viewer's requests.
func TestBridgesDoNotShareCookies(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "secret", Path: "/"})
		default:
			if cookie, err := r.Cookie("session"); err == nil {
				_, _ = w.Write([]byte(cookie.Value))
			}
		}
	}))
	defer origin.Close()

	base, err := url.Parse(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	viewer, other := NewBridge(base, nil), NewBridge(base, nil)

	if _, err := viewer.client.Get(origin.URL + "/login"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if got := fetch(t, viewer, origin.URL+"/whoami"); got != "secret" {
		t.Fatalf("the viewer that logged in lost its session: got %q", got)
	}
	if got := fetch(t, other, origin.URL+"/whoami"); got != "" {
		t.Fatalf("another viewer inherited the session cookie: got %q", got)
	}
}

// A dropped connection means a new bridge, and so a jar that starts empty.
func TestNewBridgeStartsWithAnEmptyJar(t *testing.T) {
	base, err := url.Parse("http://127.0.0.1:3000")
	if err != nil {
		t.Fatal(err)
	}
	bridge := NewBridge(base, nil)
	if cookies := bridge.client.Jar.Cookies(base); len(cookies) != 0 {
		t.Fatalf("expected no cookies on a fresh bridge, got %d", len(cookies))
	}
}

func fetch(t *testing.T, bridge *Bridge, target string) string {
	t.Helper()
	response, err := bridge.client.Get(target)
	if err != nil {
		t.Fatalf("get %s: %v", target, err)
	}
	defer response.Body.Close()
	body := make([]byte, 64)
	n, _ := response.Body.Read(body)
	return string(body[:n])
}
