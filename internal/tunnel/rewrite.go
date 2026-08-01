package tunnel

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

var (
	localOrigin  = regexp.MustCompile(`https?://(?:localhost|127\.0\.0\.1)(?::\d+)?`)
	rootHTMLPath = regexp.MustCompile(`(?i)(\b(?:src|href|action)=["'])/([^/])`)
)

// prepareHTML rewrites an HTML document for the client's path prefix:
// absolute localhost origins are stripped, root-relative src/href/action
// attributes and JS imports gain the prefix, and the PunchPage runtime
// script tag is injected at the start of <head>.
func prepareHTML(body []byte, prefix string) []byte {
	text := localOrigin.ReplaceAllString(string(body), "")
	if prefix != "" {
		text = rootHTMLPath.ReplaceAllString(text, `${1}`+prefix+`/${2}`)
		text = string(rewriteJavaScript([]byte(text), prefix))
	}
	scope := prefix
	if index := strings.Index(scope, "__punchpage__/"); index >= 0 {
		scope = scope[:index]
	}
	prefixJSON, _ := json.Marshal(prefix)
	runtimeTag := `<script>window.__PUNCHPAGE_PREFIX__=` + string(prefixJSON) + `</script><script src="` + scope + `__punchpage_runtime__.js"></script>`
	lower := strings.ToLower(text)
	if index := strings.Index(lower, "<head"); index >= 0 {
		if end := strings.Index(text[index:], ">"); end >= 0 {
			position := index + end + 1
			return []byte(text[:position] + runtimeTag + text[position:])
		}
	}
	return []byte(runtimeTag + text)
}

// rewriteJavaScript prefixes root-relative module specifiers in import
// statements and dynamic imports.
func rewriteJavaScript(body []byte, prefix string) []byte {
	if prefix == "" {
		return body
	}
	text := string(body)
	for _, marker := range []string{`from "`, `from '`, `import "`, `import '`, `import("`, `import('`} {
		text = strings.ReplaceAll(text, marker+`/`, marker+prefix+`/`)
	}
	return []byte(text)
}

// rewriteCSS prefixes root-relative url() references.
func rewriteCSS(body []byte, prefix string) []byte {
	if prefix == "" {
		return body
	}
	text := strings.ReplaceAll(string(body), "url(/", "url("+prefix+"/")
	text = strings.ReplaceAll(text, `url("/`, `url("`+prefix+`/`)
	text = strings.ReplaceAll(text, `url('/`, `url('`+prefix+`/`)
	return []byte(text)
}

// firstHeader returns the first value of a header by case-insensitive name.
func firstHeader(headers map[string][]string, wanted string) string {
	for name, values := range headers {
		if strings.EqualFold(name, wanted) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// rewriteLocation converts absolute redirects that target the local origin
// into relative ones so the browser stays on the tunneled site.
func rewriteLocation(base *url.URL, location string) string {
	parsed, err := url.Parse(location)
	if err != nil || !parsed.IsAbs() {
		return location
	}
	if parsed.Host == base.Host {
		return parsed.RequestURI()
	}
	return location
}

// isHopHeader reports whether name is a hop-by-hop header that must not be
// forwarded.
func isHopHeader(name string) bool {
	switch strings.ToLower(name) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}
