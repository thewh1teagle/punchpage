// Command fixture serves the built-in demo site on a fixed port so the
// Playwright e2e test can tunnel it and assert every check passes.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/thewh1teagle/punchpage/internal/demo"
)

func main() {
	port := flag.Int("port", 8213, "listen port")
	flag.Parse()

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	log.Printf("fixture listening on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, demo.NewHandler()))
}
