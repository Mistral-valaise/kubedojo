package main

import (
	"flag"
	"log/slog"

	"github.com/Mistral-valaise/kubedojo/internal/api"
)

// aiFlags are the flags of the assistant. `serve` and `up` have them. There
// are none yet: the AI backend adds --ai, --ai-provider, --ai-model,
// --ai-base-url and --ai-trace-content here.
type aiFlags struct{}

// registerAIFlags registers the flags of the assistant on fl.
func registerAIFlags(fl *flag.FlagSet) *aiFlags {
	return &aiFlags{}
}

// tutor returns the assistant the flags and the environment configure, or nil
// if there is none.
func (f *aiFlags) tutor(log *slog.Logger) api.Tutor {
	return newTutor(log)
}
