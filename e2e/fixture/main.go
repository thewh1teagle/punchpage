// Command fixture serves the built-in demo site on a fixed port so the
// Playwright e2e test can tunnel it and assert every check passes.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/thewh1teagle/punchpage/internal/demo"
)

func main() {
	port := flag.Int("port", 8213, "listen port")
	flag.Parse()

	// A tiny session API the demo page never calls on its own, so a test can
	// prove that one viewer's login does not reach another viewer.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "secret", Path: "/"})
		io.WriteString(w, "logged in")
	})
	mux.HandleFunc("/api/whoami", func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie("session"); err == nil {
			io.WriteString(w, cookie.Value)
		}
	})
	mux.Handle("/", demo.NewHandler())

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	log.Printf("fixture listening on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
