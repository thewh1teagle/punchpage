package main

import (
	"strings"
	"testing"
)

func TestMakeShareURLKeepsSecretInFragment(t *testing.T) {
	result, err := makeShareURL("https://thewh1teagle.github.io/punchpage/", "room", "secret", []string{"wss://nos.lol"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "#") || strings.Contains(strings.Split(result, "#")[0], "secret") {
		t.Fatalf("secret was not confined to fragment: %s", result)
	}
}

func TestMakeShareURLRejectsPlainHTTP(t *testing.T) {
	if _, err := makeShareURL("http://example.com/", "room", "secret", nil); err == nil {
		t.Fatal("expected non-localhost http page URL to be rejected")
	}
}

func TestSplitRelaysKeepsOnlySecureWebSockets(t *testing.T) {
	relays := splitRelays(" wss://nos.lol , ws://insecure.example, https://not-a-relay, wss://relay.damus.io")
	if len(relays) != 2 || relays[0] != "wss://nos.lol" || relays[1] != "wss://relay.damus.io" {
		t.Fatalf("unexpected relays: %v", relays)
	}
}
