package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/kubedojo/kubedojo/internal/api"
	"github.com/kubedojo/kubedojo/internal/session"
)

// These hooks are filled in by the telemetry, hosting and tutor milestones.

func setupTelemetry(ctx context.Context, log *slog.Logger) (func(), session.Hooks, func(int), *slog.Logger, error) {
	return func() {}, session.Hooks{}, nil, log, nil
}

func registerGauges(*session.Manager) {}

func wrapHTTP(h http.Handler) http.Handler { return h }

func hostProvider(kind string, log *slog.Logger) (session.HostProvider, error) {
	if kind == "local" {
		return session.LocalHosts(), nil
	}
	return nil, fmt.Errorf("sandbox host %q is not available in this build", kind)
}

func newTutor(log *slog.Logger) api.Tutor { return nil }
