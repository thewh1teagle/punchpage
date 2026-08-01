package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSignalEncryptionRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	plaintext := []byte(`{"type":"offer"}`)
	encoded, err := encryptSignal(key, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decryptSignal(key, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, plaintext) {
		t.Fatalf("round trip changed plaintext: %q", decoded)
	}
}

func TestPrepareHTMLForGitHubPagesScope(t *testing.T) {
	prefix := "/punchpage/__punchpage__/token"
	result := string(prepareHTML([]byte(`<html><head><script type="module">import Refresh from "/@react-refresh"</script><script type="module" src="/src/main.js"></script></head></html>`), prefix))
	for _, wanted := range []string{
		`window.__PUNCHPAGE_PREFIX__="/punchpage/__punchpage__/token"`,
		`src="/punchpage/__punchpage_runtime__.js"`,
		`src="/punchpage/__punchpage__/token/src/main.js"`,
		`from "/punchpage/__punchpage__/token/@react-refresh"`,
	} {
		if !strings.Contains(result, wanted) {
			t.Fatalf("prepared HTML missing %q: %s", wanted, result)
		}
	}
}

func TestMakeShareURLKeepsSecretInFragment(t *testing.T) {
	result, err := makeShareURL("https://thewh1teagle.github.io/punchpage/", "room", "secret", []string{"wss://nos.lol"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "#") || strings.Contains(strings.Split(result, "#")[0], "secret") {
		t.Fatalf("secret was not confined to fragment: %s", result)
	}
}
