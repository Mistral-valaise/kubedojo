package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
	"github.com/Mistral-valaise/kubedojo/internal/ai/netguard"
	"github.com/Mistral-valaise/kubedojo/internal/ai/providers"
	"github.com/Mistral-valaise/kubedojo/internal/ai/settings"
	"github.com/Mistral-valaise/kubedojo/internal/api"
	"github.com/Mistral-valaise/kubedojo/internal/assistant"
	"github.com/Mistral-valaise/kubedojo/internal/checker"
)

// aiFlags are the flags of the assistant. `serve` and `up` have them.
//
// There is deliberately no flag for the API key: a flag shows in `ps`. The
// key comes from the environment or, on a local server, from the settings.
type aiFlags struct {
	fl *flag.FlagSet
}

// oneOf is a flag that takes one of a few words. A wrong word fails when the
// flags are parsed, before anything is started.
type oneOf struct {
	value   string
	allowed []string
}

func (o *oneOf) String() string { return o.value }

func (o *oneOf) Set(v string) error {
	v = strings.ToLower(strings.TrimSpace(v))
	if !slices.Contains(o.allowed, v) {
		return fmt.Errorf("must be one of: %s", strings.Join(o.allowed, ", "))
	}
	o.value = v
	return nil
}

// registerAIFlags registers the flags of the assistant on fl. Their defaults
// are empty on purpose: what a flag does not set comes from the environment,
// then from the settings file, then from a default (docs/assistant.md).
func registerAIFlags(fl *flag.FlagSet) *aiFlags {
	fl.Bool("ai", false, "switch the AI assistant on or off (env KUBEDOJO_AI; on by default when ANTHROPIC_API_KEY is set)")
	fl.Var(&oneOf{allowed: []string{settings.ProviderAnthropic, settings.ProviderOpenAI}}, "ai-provider",
		"model provider: anthropic | openai (any OpenAI-compatible server: OpenAI, Ollama, LM Studio, vLLM, OpenRouter) (env KUBEDOJO_AI_PROVIDER)")
	fl.String("ai-model", "", "model of the assistant (env KUBEDOJO_AI_MODEL; default for anthropic: "+settings.DefaultAnthropicModel+")")
	fl.String("ai-base-url", "", "base URL of the provider's API, e.g. http://localhost:11434/v1 for Ollama (env KUBEDOJO_AI_BASE_URL)")
	fl.Var(&oneOf{value: settings.TraceAuto, allowed: []string{settings.TraceAuto, settings.TraceOn, settings.TraceOff}}, "ai-trace-content",
		"record prompts and answers in traces: auto | on | off; auto records them only for the bundled local observability stack (env KUBEDOJO_AI_TRACE_CONTENT)")
	return &aiFlags{fl: fl}
}

// given returns the assistant's flags that were set on the command line. A
// flag that was set wins over everything else and locks its setting.
func (f *aiFlags) given() map[string]string {
	out := map[string]string{}
	if f.fl == nil { // flags that were never registered: nothing was given
		return out
	}
	f.fl.Visit(func(fl *flag.Flag) {
		switch fl.Name {
		case "ai", "ai-provider", "ai-model", "ai-base-url", "ai-trace-content":
			out[fl.Name] = fl.Value.String()
		}
	})
	return out
}

// hosted reports whether the server is a shared deployment: behind a proxy
// that names the user, or with sandboxes in a cluster. Its assistant is
// configured by the deployment and cannot be changed at run time.
func hosted(p serverParts) bool {
	return p.userHeader != "" || p.sandboxHost == "kubernetes"
}

// assistant builds the assistant the flags, the environment and (on a local
// server) the settings file configure. It always returns a service: one that
// is switched off says so through the API and can be switched on there.
func (f *aiFlags) assistant(p serverParts) *assistant.Service {
	factory := providers.Factory{Guard: netguard.Policy{SelfAddr: p.addr}}
	resolver := settings.New(settings.Inputs{Flags: f.given(), LookupEnv: os.LookupEnv, Hosted: hosted(p), Dir: p.dataDir, CheckURL: factory.CheckURL})
	if err := resolver.LoadError(); err != nil {
		p.log.Warn("assistant: the settings file is ignored", "err", err)
	}
	eff := resolver.Effective()
	// Whether traces may hold prompts and answers is decided by flags and
	// environment alone, so it is settled here for the life of the server.
	content, _ := assistant.TraceContent(eff, p.localStack)
	var hooks assistant.Hooks
	if p.telemetry != nil {
		// A call to the model is a span and metric samples, and so is the
		// HTTP request below it. The provider gets no trace context. Where
		// prompts and answers are recorded, they are redacted like the
		// output of a check.
		p.telemetry.Redact = checker.Redact
		factory.Wrap = func(pr ai.Provider) ai.Provider {
			return p.telemetry.InstrumentProvider(pr, func() bool { return content })
		}
		factory.Transport = p.telemetry.ModelTransport
		hooks = assistant.Hooks{Request: p.telemetry.AssistantRequest, Context: p.telemetry.AssistantContext}
	}
	holder := ai.NewHolder(factory.Build)
	_ = holder.Apply(eff) // why there is no provider is logged below and shown in the settings
	svc := assistant.New(assistant.Config{Holder: holder, Settings: resolver, Factory: factory, Store: p.store, Sessions: p.sessions,
		Catalog: p.catalog, Log: p.log, Render: api.Render, LocalStack: p.localStack, Hooks: hooks})
	svc.LogState()
	return svc
}
