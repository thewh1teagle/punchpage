package main

import (
	"flag"
	"slices"
	"strings"
	"testing"
)

func TestResolveTarget(t *testing.T) {
	cases := []struct {
		name    string
		arg     string
		want    string
		wantErr bool
	}{
		{name: "empty uses default", arg: "", want: defaultTarget},
		{name: "bare port", arg: "3000", want: "http://127.0.0.1:3000"},
		{name: "colon port", arg: ":3000", want: "http://127.0.0.1:3000"},
		{name: "full url", arg: "http://localhost:8080", want: "http://localhost:8080"},
		{name: "https url with path", arg: "https://example.test/app", want: "https://example.test/app"},
		{name: "host and port", arg: "localhost:8080", want: "http://localhost:8080"},
		{name: "host only", arg: "localhost", want: "http://localhost"},
		{name: "ipv4 and port", arg: "192.168.1.5:5000", want: "http://192.168.1.5:5000"},
		{name: "port out of range", arg: "99999", wantErr: true},
		{name: "zero port", arg: "0", wantErr: true},
		{name: "colon without port", arg: ":", wantErr: true},
		{name: "colon with non-numeric port", arg: ":abc", wantErr: true},
		{name: "host with bad port", arg: "localhost:99999", wantErr: true},
		{name: "scheme without host", arg: "http://", wantErr: true},
		{name: "garbage with spaces", arg: "not a url", wantErr: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := resolveTarget(testCase.arg)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("resolveTarget(%q) = %q, want error", testCase.arg, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveTarget(%q) failed: %v", testCase.arg, err)
			}
			if got != testCase.want {
				t.Fatalf("resolveTarget(%q) = %q, want %q", testCase.arg, got, testCase.want)
			}
		})
	}
}

func TestSplitArgs(t *testing.T) {
	newSet := func() *flag.FlagSet {
		set := flag.NewFlagSet("punch", flag.ContinueOnError)
		set.String("interface", "", "")
		set.String("target", "", "")
		set.Bool("verbose", false, "")
		return set
	}
	cases := []struct {
		name            string
		args            []string
		wantFlags       []string
		wantPositionals []string
	}{
		{name: "no args"},
		{name: "positional only", args: []string{"3000"}, wantPositionals: []string{"3000"}},
		{
			name:            "flag after positional",
			args:            []string{"3000", "--interface", "en0"},
			wantFlags:       []string{"--interface", "en0"},
			wantPositionals: []string{"3000"},
		},
		{
			name:            "flag before positional",
			args:            []string{"-interface", "en0", "demo"},
			wantFlags:       []string{"-interface", "en0"},
			wantPositionals: []string{"demo"},
		},
		{
			name:            "equals form keeps one token",
			args:            []string{"--interface=en0", "3000"},
			wantFlags:       []string{"--interface=en0"},
			wantPositionals: []string{"3000"},
		},
		{
			name:            "bool flag does not eat the positional",
			args:            []string{"--verbose", "3000"},
			wantFlags:       []string{"--verbose"},
			wantPositionals: []string{"3000"},
		},
		{
			name:            "double dash forces positionals",
			args:            []string{"--", "--interface"},
			wantPositionals: []string{"--interface"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			flags, positionals := splitArgs(newSet(), testCase.args)
			if !slices.Equal(flags, testCase.wantFlags) {
				t.Fatalf("flags = %v, want %v", flags, testCase.wantFlags)
			}
			if !slices.Equal(positionals, testCase.wantPositionals) {
				t.Fatalf("positionals = %v, want %v", positionals, testCase.wantPositionals)
			}
		})
	}
}

func TestMakeShareURLKeepsSecretInFragment(t *testing.T) {
	result, err := makeShareURL("https://punchpage.pages.dev/", "room", "secret", []string{"wss://nos.lol"})
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
