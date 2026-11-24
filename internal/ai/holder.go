package ai

import (
	"sync"
	"sync/atomic"

	"github.com/Mistral-valaise/kubedojo/internal/ai/settings"
)

// Builder makes the provider a configuration describes.
type Builder func(settings.Effective) (Provider, error)

// Holder holds the provider that is in use and swaps it when the settings
// change. A call that is under way keeps the provider it started with; the
// next call gets the new one. No restart is needed.
type Holder struct {
	cur   atomic.Pointer[state]
	build Builder
	// apply serialises Apply, so that two changes land in the order made.
	apply sync.Mutex
}

type state struct {
	provider Provider
	eff      settings.Effective
	problem  string
}

// NewHolder returns a holder without a provider. Apply gives it one.
func NewHolder(build Builder) *Holder {
	h := &Holder{build: build}
	h.cur.Store(&state{problem: "The assistant is not configured."})
	return h
}

// Current returns the provider in use and the settings it was built from.
// The provider is nil while the assistant is switched off or not usable;
// Problem says why.
func (h *Holder) Current() (Provider, settings.Effective) {
	s := h.cur.Load()
	return s.provider, s.eff
}

// Problem says in one sentence why there is no provider. It is empty when
// there is one.
func (h *Holder) Problem() string { return h.cur.Load().problem }

// Apply puts new settings in force. If no provider can be built from them
// the holder has none afterwards, and the error says why: the settings stay,
// a later Apply with the same settings may succeed.
func (h *Holder) Apply(eff settings.Effective) error {
	h.apply.Lock()
	defer h.apply.Unlock()
	next := &state{eff: eff}
	var err error
	switch {
	case eff.Problem != "":
		next.problem = eff.Problem
	case !eff.Enabled.Value:
		next.problem = "The assistant is switched off."
	default:
		next.provider, err = h.build(eff)
		if err != nil {
			next.provider = nil
			next.problem = AsError(err).Message
		}
	}
	h.cur.Store(next)
	return err
}
