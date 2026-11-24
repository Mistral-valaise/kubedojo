package httpsec

import (
	"net"
	"net/http"
	"strings"
)

// exemptPaths are answered whatever the Host is: the kubelet probes them with
// the Pod's IP address as Host. They reveal nothing and change nothing.
var exemptPaths = map[string]bool{"/api/health": true, "/api/ready": true}

// hostname returns the host of a Host header or of a listen address in the
// form the allowlist compares: without the port, without the brackets of an
// IPv6 literal, without a trailing dot, in lower case.
func hostname(hostport string) string {
	h := strings.TrimSpace(hostport)
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	} else {
		h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	}
	return strings.ToLower(strings.TrimSuffix(h, "."))
}

// originHost returns the host of an origin given as "host[:port]" or as
// "scheme://host[:port]".
func originHost(origin string) string {
	if _, rest, ok := strings.Cut(origin, "://"); ok {
		origin = rest
	}
	origin, _, _ = strings.Cut(origin, "/")
	return hostname(origin)
}

// wildcardAddr reports whether a listen address means every interface.
func wildcardAddr(addr string) bool {
	h := hostname(addr)
	if h == "" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsUnspecified()
}

// allowlist decides which Host values are accepted.
type allowlist struct {
	exact    map[string]bool
	suffixes []string // ".example.com": every subdomain of example.com
	any      bool     // the check is off
}

// add accepts one more host: a name or an address, or "*.example.com" for the
// subdomains of example.com. A port or a scheme in the entry is ignored.
func (a *allowlist) add(entry string) {
	h := originHost(entry)
	switch {
	case h == "":
	case strings.HasPrefix(h, "*.") && !strings.Contains(h[1:], "*"):
		a.suffixes = append(a.suffixes, h[1:])
	case !strings.Contains(h, "*"):
		a.exact[h] = true
	}
}

func (a *allowlist) allows(host string) bool {
	if a.any {
		return true
	}
	h := hostname(host)
	if h == "" {
		return false
	}
	if a.exact[h] {
		return true
	}
	for _, s := range a.suffixes {
		if len(h) > len(s) && strings.HasSuffix(h, s) {
			return true
		}
	}
	return false
}

// newAllowlist builds the allowlist of a server. The second result says
// whether the list is enforced; HostAllowlist logs why when it is not.
//
// Always accepted are the names of this machine that cannot be made to point
// anywhere else: localhost, 127.0.0.1, [::1] and every name below .localhost
// (RFC 6761). The address the server listens on is accepted too, unless it
// means "every interface", and so is the host of the dev server.
func newAllowlist(cfg Config) (a *allowlist, enforce bool) {
	a = &allowlist{exact: map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}, suffixes: []string{".localhost"}}
	wildcard := wildcardAddr(cfg.ListenAddr)
	if !wildcard {
		a.exact[hostname(cfg.ListenAddr)] = true
	}
	a.add(cfg.DevOrigin)
	given := 0
	for _, e := range cfg.AllowedHosts {
		switch e = strings.TrimSpace(e); e {
		case "":
		case "*":
			a.any = true
		default:
			given++
			a.add(e)
		}
	}
	// A server on every interface without a list is what a container gets
	// that was started with nothing but `serve`: it must keep working.
	return a, !a.any && (!wildcard || given > 0)
}

// HostAllowlist rejects requests whose Host header is not one the server is
// known by, with 403. This is what stops DNS rebinding: the attacker's page
// reaches 127.0.0.1, but under the attacker's name.
//
// The list is enforced when the server listens on a specific address or when
// AllowedHosts is set. A server on all interfaces without AllowedHosts
// accepts every Host and says so once at start.
func HostAllowlist(cfg Config) func(http.Handler) http.Handler {
	log := cfg.log()
	a, enforce := newAllowlist(cfg)
	switch {
	case a.any:
		log.Warn("the Host check is off (--allowed-hosts=*); any Host header is accepted", "addr", cfg.ListenAddr)
	case !enforce:
		log.Warn("listening on all interfaces without --allowed-hosts; any Host header is accepted", "addr", cfg.ListenAddr)
	}
	if !enforce {
		return func(h http.Handler) http.Handler { return h }
	}
	var logged seen
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if a.allows(r.Host) || exemptPaths[r.URL.Path] {
				h.ServeHTTP(w, r)
				return
			}
			cfg.rejected(ReasonHost)
			host := hostname(r.Host)
			if logged.first(host) {
				log.Warn("request rejected: this Host is not allowed; add it with --allowed-hosts or KUBEDOJO_ALLOWED_HOSTS",
					"host", r.Host, "path", r.URL.Path)
			}
			if len(host) > 100 {
				host = host[:100] + "..."
			}
			writeError(w, http.StatusForbidden, `The host "`+host+`" is not allowed. If kubedojo is meant to be reached under this name, `+
				"add it with --allowed-hosts or KUBEDOJO_ALLOWED_HOSTS.")
		})
	}
}
