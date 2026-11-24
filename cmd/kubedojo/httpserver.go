package main

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/api"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
	"github.com/Mistral-valaise/kubedojo/internal/telemetry"
	"github.com/Mistral-valaise/kubedojo/internal/webui"
)

// serverParts is what cmdServe has built and parsed by the time it makes the
// API server.
type serverParts struct {
	catalog    *catalog.Catalog
	sessions   *session.Manager
	store      *store.Store
	telemetry  *telemetry.Telemetry
	log        *slog.Logger
	userHeader string
	devOrigin  string
	ai         *aiFlags
	// What the assistant and the security checks need to know about how this
	// server runs.
	local       bool   // `up`: one user on their own machine
	dataDir     string // where kubedojo keeps its files
	addr        string // the address the server listens on
	sandboxHost string // local | kubernetes
	localStack  bool   // the bundled observability stack is running
}

// newAPIServer builds the API server from its parts.
func newAPIServer(p serverParts) *api.Server {
	srv := &api.Server{Catalog: p.catalog, Sessions: p.sessions, Store: p.store, UI: webui.FS(), Log: p.log, Version: version,
		UserHeader: p.userHeader, OnTerminal: p.telemetry.Terminal, OnRating: p.telemetry.Rated, Tutor: p.ai.tutor(p.log)}
	if p.devOrigin != "" {
		srv.AllowedOrigins = []string{p.devOrigin}
	}
	return srv
}

// newHTTPServer builds the HTTP server that serves h. cmdServe serves it on a
// listener it already has; addr is that listener's address.
func newHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
}
