package tunnel

import (
	"net/url"
	"strings"
	"testing"
)

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

func TestRewriteCSSPrefixesRootURLs(t *testing.T) {
	input := `body{background:url(/bg.png) url("/a.png") url('/b.png') url(https://cdn.example/c.png)}`
	result := string(rewriteCSS([]byte(input), "/p"))
	for _, wanted := range []string{`url(/p/bg.png)`, `url("/p/a.png")`, `url('/p/b.png')`, `url(https://cdn.example/c.png)`} {
		if !strings.Contains(result, wanted) {
			t.Fatalf("rewritten CSS missing %q: %s", wanted, result)
		}
	}
}

func TestRewriteLocation(t *testing.T) {
	base, _ := url.Parse("http://127.0.0.1:3000")
	if got := rewriteLocation(base, "http://127.0.0.1:3000/login?next=%2F"); got != "/login?next=%2F" {
		t.Fatalf("local redirect not made relative: %q", got)
	}
	if got := rewriteLocation(base, "https://example.com/out"); got != "https://example.com/out" {
		t.Fatalf("external redirect changed: %q", got)
	}
	if got := rewriteLocation(base, "/already-relative"); got != "/already-relative" {
		t.Fatalf("relative redirect changed: %q", got)
	}
}

func TestFirstHeaderIsCaseInsensitive(t *testing.T) {
	headers := map[string][]string{"x-punchpage-prefix": {"/p", "/ignored"}}
	if got := firstHeader(headers, "X-PunchPage-Prefix"); got != "/p" {
		t.Fatalf("firstHeader = %q, want /p", got)
	}
	if got := firstHeader(headers, "missing"); got != "" {
		t.Fatalf("firstHeader for missing header = %q, want empty", got)
	}
}
