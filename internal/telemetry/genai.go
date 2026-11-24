package telemetry

import "github.com/Mistral-valaise/kubedojo/internal/ai"

// InstrumentProvider will wrap the provider with GenAI spans and metrics (the
// OpenTelemetry gen_ai semantic conventions). content says, per call, whether
// prompts and answers may be recorded. For now it returns p as it is.
func (t *Telemetry) InstrumentProvider(p ai.Provider, content func() bool) ai.Provider {
	return p
}
