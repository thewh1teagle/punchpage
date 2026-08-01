// Command punchpage shares a local HTTP origin peer-to-peer: it tunnels
// HTTP and WebSocket traffic over WebRTC, using encrypted Nostr events for
// signaling, and prints a GitHub Pages share URL that carries the room and
// key in its fragment.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	ossignal "os/signal"
	"strings"
	"syscall"

	"github.com/thewh1teagle/punchpage/internal/demo"
	"github.com/thewh1teagle/punchpage/internal/host"
	"github.com/thewh1teagle/punchpage/internal/signaling"
)

func main() {
	page := flag.String("page", "https://thewh1teagle.github.io/punchpage/", "GitHub Pages browser URL")
	relayList := flag.String("relays", "wss://relay.damus.io,wss://nos.lol,wss://relay.primal.net", "comma-separated public Nostr relays")
	room := flag.String("room", "", "existing room identifier (normally generated)")
	keyText := flag.String("key", "", "existing base64url signaling key (normally generated)")
	target := flag.String("target", "http://127.0.0.1:3000", "local HTTP origin")
	iface := flag.String("interface", "", "optional network interface to expose to ICE")
	args := os.Args[1:]
	demoMode := len(args) > 0 && args[0] == "demo"
	if demoMode {
		args = args[1:]
	}
	flag.CommandLine.Parse(args)

	if demoMode {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			log.Fatal(err)
		}
		go http.Serve(listener, demo.NewHandler())
		*target = "http://" + listener.Addr().String()
		fmt.Println("\nDemo mode: sharing the built-in demo site. Open the link below to watch")
		fmt.Println("every tunnel check pass, then send it to a friend.")
	}

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
	signaler, err := signaling.New(ctx, relays, *room, key)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\nPunchPage is sharing %s\n\n  %s\n\n", base, shareURL)
	log.Printf("encrypted signaling room=%s relays=%d", *room, len(relays))
	if err := host.Run(ctx, signaler, *iface, base); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}

// splitRelays parses a comma-separated relay list, keeping only wss:// URLs.
func splitRelays(value string) []string {
	var relays []string
	for relay := range strings.SplitSeq(value, ",") {
		if relay = strings.TrimSpace(relay); strings.HasPrefix(relay, "wss://") {
			relays = append(relays, relay)
		}
	}
	return relays
}

// randomToken returns size random bytes as an unpadded base64url string.
func randomToken(size int) string {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

// makeShareURL builds the browser URL with the room, key, and relays in the
// fragment so the secret never reaches the Pages server.
func makeShareURL(page, room, key string, relays []string) (string, error) {
	parsed, err := url.Parse(page)
	if err != nil {
		return "", fmt.Errorf("--page must be an absolute HTTPS URL (or localhost for development)")
	}
	localHTTP := parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1")
	if (parsed.Scheme != "https" && !localHTTP) || parsed.Host == "" {
		return "", fmt.Errorf("--page must be an absolute HTTPS URL (or localhost for development)")
	}
	parsed.Fragment = "r=" + room + "&k=" + key + "&relays=" + strings.Join(relays, ",")
	return parsed.String(), nil
}

func init() { log.SetFlags(log.LstdFlags | log.Lmicroseconds) }
