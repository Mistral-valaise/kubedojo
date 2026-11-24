// Package httpsec protects an HTTP API that has no authentication of its own
// and is meant to be reached from one machine, or through a proxy that
// authenticates.
//
// Binding to 127.0.0.1 keeps other machines out, but not the web pages the
// user visits: a browser on this machine reaches the port as well. Two things
// stand between such a page and the API:
//
//   - the Host allowlist. A page that makes its own domain resolve to
//     127.0.0.1 (DNS rebinding) is same-origin with the API as far as the
//     browser is concerned, but its requests carry that domain as Host.
//   - cross-origin protection. A page of another origin cannot read the API's
//     answers, but it can send requests that change state (a form post).
//     Requests the browser marks as cross-site are rejected.
//
// The package uses net/http only.
package httpsec

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
)

// Why a request was rejected, as OnReject reports it.
const (
	ReasonHost        = "host"
	ReasonCrossOrigin = "cross_origin"
	// ReasonContentType is reported by the code that reads request bodies,
	// not by this package: a body that is not declared as JSON.
	ReasonContentType = "content_type"
)

// Config says how the server runs.
type Config struct {
	// ListenAddr is the address the server listens on, e.g. 127.0.0.1:4400.
	ListenAddr string
	// AllowedHosts are more Host values to accept, without a port:
	// "kubedojo.example.com", "*.example.com" for every subdomain. A single
	// "*" turns the Host check off.
	AllowedHosts []string
	// DevOrigin is the origin of a UI dev server that may call the API, as a
	// host ("localhost:5173") or with its scheme ("http://localhost:5173").
	DevOrigin string
	// Log receives the warnings at start and the rejected requests. May be nil.
	Log *slog.Logger
	// OnReject is called for every rejected request with the reason. May be nil.
	OnReject func(reason string)
}

func (c Config) log() *slog.Logger {
	if c.Log != nil {
		return c.Log
	}
	return slog.New(slog.DiscardHandler)
}

func (c Config) rejected(reason string) {
	if c.OnReject != nil {
		c.OnReject(reason)
	}
}

// Chain returns all of the package's protections in the order they must run:
//
//	Recover → HostAllowlist → CrossOrigin → Headers → handler
//
// A request with a Host that is not allowed is answered before anything looks
// at it further; both checks read a few headers and nothing of the body.
// Every answer carries the security headers: Headers sets them for what
// reaches the handler, and the rejections and Recover set them on their own
// answers, with a policy that names no host.
func Chain(cfg Config) func(http.Handler) http.Handler {
	recov, hosts, cross := Recover(cfg.Log), HostAllowlist(cfg), CrossOrigin(cfg)
	return func(h http.Handler) http.Handler {
		return recov(hosts(cross(Headers(h))))
	}
}

// Recover answers a panic of the handler with 500 and logs it with its stack.
// net/http alone would close the connection without an answer and write the
// stack to the standard logger.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if v == http.ErrAbortHandler {
					panic(v) // the handler's way to drop the connection
				}
				log.ErrorContext(r.Context(), "panic in a request", "method", r.Method, "path", r.URL.Path,
					"panic", fmt.Sprint(v), "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal error")
			}()
			h.ServeHTTP(w, r)
		})
	}
}

// writeError answers with {"error": msg}, the format of the API, and with the
// headers every answer carries.
func writeError(w http.ResponseWriter, code int, msg string) {
	setHeaders(w.Header(), "")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// seen remembers a bounded number of values, to log each rejected host or
// origin once instead of once per request.
type seen struct {
	mu sync.Mutex
	m  map[string]bool
}

func (s *seen) first(v string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[v] || len(s.m) >= 64 {
		return false
	}
	if s.m == nil {
		s.m = map[string]bool{}
	}
	s.m[v] = true
	return true
}
