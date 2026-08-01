// Command punch shares a local HTTP origin peer-to-peer: it tunnels
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
	"strconv"
	"strings"
	"syscall"

	"github.com/thewh1teagle/punchpage/internal/demo"
	"github.com/thewh1teagle/punchpage/internal/host"
	"github.com/thewh1teagle/punchpage/internal/signaling"
)

// defaultTarget is the local origin shared when no target is given.
const defaultTarget = "http://127.0.0.1:3000"

func main() {
	page := flag.String("page", "https://punchpage.pages.dev/", "browser client URL")
	relayList := flag.String("relays", "wss://relay.damus.io,wss://nos.lol,wss://relay.primal.net", "comma-separated public Nostr relays")
	room := flag.String("room", "", "existing room identifier (normally generated)")
	keyText := flag.String("key", "", "existing base64url signaling key (normally generated)")
	targetFlag := flag.String("target", "", "deprecated: pass the target as a positional argument instead")
	iface := flag.String("interface", "", "optional network interface to expose to ICE")
	flag.Usage = usage

	flagArgs, positionals := splitArgs(flag.CommandLine, os.Args[1:])
	flag.CommandLine.Parse(flagArgs)
	if len(positionals) > 1 {
		log.Fatalf("expected at most one target argument, got %d: %s", len(positionals), strings.Join(positionals, " "))
	}

	var positional string
	if len(positionals) == 1 {
		positional = positionals[0]
	}
	demoMode := positional == "demo"

	target := defaultTarget
	switch {
	case demoMode:
		// The demo server picks its own port below.
	case positional != "":
		resolved, err := resolveTarget(positional)
		if err != nil {
			log.Fatal(err)
		}
		target = resolved
	case *targetFlag != "":
		fmt.Fprintln(os.Stderr, "note: --target is deprecated; run `punch <port|url>` instead")
		resolved, err := resolveTarget(*targetFlag)
		if err != nil {
			log.Fatal(err)
		}
		target = resolved
	}

	if demoMode {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			log.Fatal(err)
		}
		go http.Serve(listener, demo.NewHandler())
		target = "http://" + listener.Addr().String()
		fmt.Println("\nDemo mode: sharing the built-in demo site. Open the link below to watch")
		fmt.Println("every tunnel check pass, then send it to a friend.")
	}

	base, err := url.Parse(target)
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

// usage prints the short help text ahead of the flag defaults.
func usage() {
	fmt.Fprint(flag.CommandLine.Output(), `punch — share a local web app peer-to-peer.

Usage:
  punch [target] [flags]

Targets:
  punch                        share `+defaultTarget+`
  punch 3000                   share http://127.0.0.1:3000
  punch :3000                  same as above
  punch localhost:8080         share http://localhost:8080
  punch http://localhost:8080  share that URL as-is
  punch demo                   share the built-in demo site

Flags:
`)
	flag.PrintDefaults()
}

// splitArgs separates flags from positional arguments so that flags may appear
// anywhere on the command line: the stdlib flag package otherwise stops parsing
// at the first positional argument.
func splitArgs(set *flag.FlagSet, args []string) (flags, positionals []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return flags, append(positionals, args[i+1:]...)
		}
		if len(arg) < 2 || arg[0] != '-' {
			positionals = append(positionals, arg)
			continue
		}
		flags = append(flags, arg)
		name := strings.TrimLeft(arg, "-")
		if strings.Contains(name, "=") {
			continue
		}
		// A value-taking flag written as "--name value" consumes the next token.
		if found := set.Lookup(name); found != nil && !isBoolFlag(found) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return flags, positionals
}

// isBoolFlag reports whether a flag is satisfied without a separate value.
func isBoolFlag(f *flag.Flag) bool {
	boolean, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && boolean.IsBoolFlag()
}

// resolveTarget turns a user-supplied target into an absolute origin URL:
// a bare port or ":port" means loopback, an absolute URL is used as-is, and
// anything else is treated as host[:port] over plain HTTP.
func resolveTarget(arg string) (string, error) {
	if arg == "" {
		return defaultTarget, nil
	}
	if strings.HasPrefix(arg, ":") || isDigits(arg) {
		port, ok := parsePort(strings.TrimPrefix(arg, ":"))
		if !ok {
			return "", fmt.Errorf("invalid target %q: expected a port between 1 and 65535", arg)
		}
		return "http://127.0.0.1:" + port, nil
	}
	if parsed, err := url.Parse(arg); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return arg, nil
	}
	if strings.Contains(arg, "://") {
		return "", fmt.Errorf("invalid target %q: URL is missing a host", arg)
	}
	parsed, err := url.Parse("http://" + arg)
	if err != nil || parsed.Hostname() == "" {
		return "", fmt.Errorf("invalid target %q: expected a port, host:port, or URL", arg)
	}
	if raw := parsed.Port(); raw != "" {
		if _, ok := parsePort(raw); !ok {
			return "", fmt.Errorf("invalid target %q: port must be between 1 and 65535", arg)
		}
	}
	return parsed.String(), nil
}

// isDigits reports whether text is a non-empty run of ASCII digits.
func isDigits(text string) bool {
	if text == "" {
		return false
	}
	return strings.IndexFunc(text, func(r rune) bool { return r < '0' || r > '9' }) < 0
}

// parsePort reports whether text is a valid TCP port number.
func parsePort(text string) (string, bool) {
	number, err := strconv.Atoi(text)
	if err != nil || number < 1 || number > 65535 || strings.HasPrefix(text, "+") {
		return "", false
	}
	return strconv.Itoa(number), true
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
