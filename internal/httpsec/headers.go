package httpsec

import (
	"net/http"
	"regexp"
	"strings"
)

// The policy of the UI: everything comes from the server itself. Styles may
// be inline, because React writes style attributes and xterm.js adds <style>
// elements; scripts may not, and nothing is evaluated from a string.
const (
	cspHead = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; font-src 'self' data:; connect-src 'self'"
	cspTail = "; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'"
)

// cspHost is a Host header that can stand in a policy as it is: a name or an
// IPv4 address with an optional port. The header may hold more than that
// (quotes, semicolons), and the grammar of a policy has no IPv6 literals.
var cspHost = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?(:[0-9]{1,5})?$`)

// csp returns the Content-Security-Policy for a page served under host. The
// terminal is a WebSocket to the same host; it is named explicitly because
// not every browser lets 'self' cover ws: and wss:.
func csp(host string) string {
	if !cspHost.MatchString(host) {
		return cspHead + cspTail
	}
	h := strings.ToLower(host)
	return cspHead + " ws://" + h + " wss://" + h + cspTail
}

func setHeaders(h http.Header, host string) {
	h.Set("Content-Security-Policy", csp(host))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
}

// Headers sets the security headers on every response: a Content-Security-
// Policy that keeps injected markup from running scripts or sending data
// away, no framing by other pages, no content sniffing, no referrer, and no
// reading of the responses by other origins.
//
// In Chain it runs after HostAllowlist: the Host it puts into the policy is
// one the server accepted.
func Headers(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setHeaders(w.Header(), r.Host)
		h.ServeHTTP(w, r)
	})
}
