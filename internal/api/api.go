// Package api exposes kubedojo over HTTP: REST for content and sessions, a
// WebSocket for the terminal, and the embedded single-page UI.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/assistant"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

type Server struct {
	Catalog  *catalog.Catalog
	Sessions *session.Manager
	Store    *store.Store
	UI       fs.FS
	Log      *slog.Logger
	Version  string
	// Assistant is the AI assistant. nil means the server has none: its
	// endpoints then answer that it is not configured.
	Assistant *assistant.Service
	// UserHeader names a trusted reverse-proxy header carrying the user
	// identity (hosted mode behind an OIDC proxy). Empty = single local user.
	UserHeader string
	// AllowedOrigins are extra WebSocket origin patterns (dev server).
	AllowedOrigins []string
	// OnRating is called when a learner rates a question.
	OnRating func(ctx context.Context, track, question string, stars int)
	// OnTerminal is called when a terminal connects (delta +1) or leaves (-1).
	OnTerminal func(delta int)

	draining atomic.Bool
}

// Drain makes the server report that it is not ready, so that a load balancer
// stops sending requests while it shuts down.
func (s *Server) Drain() { s.draining.Store(true) }

// ready answers a readiness probe: the database answers and the server is not
// shutting down. /api/health stays a pure liveness check.
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if s.draining.Load() {
		writeErr(w, 503, "shutting down")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.Ping(ctx); err != nil {
		writeErr(w, 503, "the database does not answer")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ready"})
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/ready", s.ready)
	mux.HandleFunc("GET /api/config", s.config)
	mux.HandleFunc("GET /api/path", s.path)
	mux.HandleFunc("GET /api/tracks", s.tracks)
	mux.HandleFunc("GET /api/tracks/{track}", s.track)
	mux.HandleFunc("GET /api/tracks/{track}/lessons/{domain}", s.lesson)
	mux.HandleFunc("GET /api/tracks/{track}/progress", s.progress)
	mux.HandleFunc("POST /api/tracks/{track}/questions/{q}/rating", s.rate)
	mux.HandleFunc("GET /api/sessions", s.listSessions)
	mux.HandleFunc("POST /api/sessions", s.createSession)
	mux.HandleFunc("GET /api/sessions/{id}", s.getSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.deleteSession)
	mux.HandleFunc("POST /api/sessions/{id}/start", s.startSession)
	mux.HandleFunc("POST /api/sessions/{id}/finish", s.finishSession)
	mux.HandleFunc("GET /api/sessions/{id}/terminal", s.terminal)
	mux.HandleFunc("GET /api/sessions/{id}/questions/{q}", s.getQuestion)
	mux.HandleFunc("PATCH /api/sessions/{id}/questions/{q}", s.patchQuestion)
	mux.HandleFunc("POST /api/sessions/{id}/questions/{q}/prepare", s.prepare)
	mux.HandleFunc("POST /api/sessions/{id}/questions/{q}/check", s.check)
	mux.HandleFunc("GET /api/sessions/{id}/questions/{q}/hints/{n}", s.hint)
	mux.HandleFunc("GET /api/sessions/{id}/questions/{q}/solution", s.solution)
	s.registerAssistant(mux)
	s.registerSettings(mux)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeErr(w, 404, "not found") })
	mux.Handle("/", s.spa())
	return mux
}

func (s *Server) spa() http.Handler {
	fileServer := http.FileServer(http.FS(s.UI))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.UI, p); err != nil {
			index, err := fs.ReadFile(s.UI, "index.html")
			if err != nil {
				http.Error(w, "UI not built. Run `make web` (or use a release binary).", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(index)
			return
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, session.ErrNotFound):
		writeErr(w, 404, "not found")
	case errors.Is(err, session.ErrConflict):
		writeErr(w, 409, err.Error())
	case errors.Is(err, session.ErrInvalid):
		writeErr(w, 400, err.Error())
	case errors.Is(err, session.ErrLimit):
		writeErr(w, 429, "Too many active sessions. End one before starting another.")
	default:
		// With the request's context the line carries its trace.
		s.Log.ErrorContext(r.Context(), "request failed", "route", r.Pattern, "err", err)
		writeErr(w, 500, err.Error())
	}
}

func (s *Server) owner(r *http.Request) string {
	if s.UserHeader == "" {
		return ""
	}
	return r.Header.Get(s.UserHeader)
}

// session returns the session if it belongs to the caller.
func (s *Server) session(w http.ResponseWriter, r *http.Request) *session.Session {
	v, err := s.Sessions.View(r.PathValue("id"))
	if err == nil && v.Owner != s.owner(r) {
		err = session.ErrNotFound
	}
	if err != nil {
		s.fail(w, r, err)
		return nil
	}
	return v
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	var st assistant.Status
	if s.Assistant != nil {
		st = s.Assistant.Status()
	}
	// "tutor" is what clients before 0.5 look at: whether asking is possible.
	writeJSON(w, 200, map[string]any{"version": s.Version, "tutor": st.Enabled && st.Configured, "assistant": st,
		"user": s.owner(r), "maxTerminals": s.Sessions.MaxTerminals()})
}
