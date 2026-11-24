package httpsec

import (
	"net/http"
	"strings"
)

// trustedOrigins returns the Origin header values a dev server sends. The dev
// origin is configured as a host pattern for the WebSocket ("localhost:5173");
// an Origin header has a scheme. A pattern with a wildcard names no single
// origin and is not trusted here.
func trustedOrigins(dev string) []string {
	dev = strings.TrimSuffix(strings.TrimSpace(dev), "/")
	switch {
	case dev == "" || strings.Contains(dev, "*"):
		return nil
	case strings.Contains(dev, "://"):
		return []string{dev}
	}
	return []string{"http://" + dev, "https://" + dev}
}

// CrossOrigin rejects requests that change state and come from another
// origin, with 403. It is net/http's CrossOriginProtection: a request with
// an unsafe method (POST, PATCH, DELETE ...) passes if the browser marks it
// as same-origin (Sec-Fetch-Site), or, for a browser that does not send that
// header, if the host of its Origin is the Host it was sent to. A request
// with neither header does not come from a browser and passes.
//
// GET requests always pass, and with them the upgrade to the terminal's
// WebSocket: that handler checks the Origin itself.
func CrossOrigin(cfg Config) func(http.Handler) http.Handler {
	log := cfg.log()
	cop := http.NewCrossOriginProtection()
	for _, o := range trustedOrigins(cfg.DevOrigin) {
		if err := cop.AddTrustedOrigin(o); err != nil {
			log.Warn("the dev origin is not an origin; cross-origin requests from it are rejected", "origin", cfg.DevOrigin, "err", err)
		}
	}
	var logged seen
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.rejected(ReasonCrossOrigin)
		if origin := r.Header.Get("Origin"); logged.first(origin) {
			log.Warn("request rejected: it comes from another origin", "origin", origin, "host", r.Host,
				"method", r.Method, "path", r.URL.Path)
		}
		writeError(w, http.StatusForbidden, "cross-origin request rejected")
	}))
	return cop.Handler
}
