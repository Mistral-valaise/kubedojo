package main

import (
	"log/slog"
	"os"

	"github.com/kubedojo/kubedojo/internal/api"
	"github.com/kubedojo/kubedojo/internal/tutor"
)

// newTutor enables the AI tutor when an Anthropic API key is configured.
func newTutor(log *slog.Logger) api.Tutor {
	if os.Getenv("ANTHROPIC_API_KEY") == "" && os.Getenv("ANTHROPIC_AUTH_TOKEN") == "" {
		return nil
	}
	t := tutor.New(os.Getenv("KUBEDOJO_TUTOR_MODEL"))
	log.Info("AI tutor enabled", "model", t.Model())
	return t
}
