package assistant

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/checker"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/span"
)

// Stages of gathering the context, as the stream reports them.
const (
	StageChecks      = "checks"
	StageSnapshot    = "snapshot"
	StageDiagnostics = "diagnostics"
)

// Time and size limits of what is run in the sandbox for one message.
const (
	snapshotTimeout    = 10 * time.Second
	diagnosticTimeout  = 5 * time.Second
	diagnosticsTimeout = 10 * time.Second
	snapshotSize       = 6 << 10
	contextTTL         = 30 * time.Second
)

const clusterSnapshot = `
echo "## nodes"; kubectl get nodes -o wide 2>&1 | head -10
echo "## non-running pods"; kubectl get pods -A --field-selector=status.phase!=Running 2>&1 | head -20
echo "## recent warning events"; kubectl get events -A --field-selector=type=Warning --sort-by=.lastTimestamp 2>&1 | tail -15
`

// hostSnapshot describes a Linux host: there is no kubectl on it. It looks at
// what an exercise owns by convention, /opt/course/<question id>/.
const hostSnapshot = `
echo "## exercise directory /opt/course/$KD_QUESTION"; ls -la "/opt/course/$KD_QUESTION" 2>&1 | head -30
echo "## processes"; ps -eo user,pid,ppid,stat,args --sort=pid 2>&1 | head -30
echo "## listening sockets"; ss -ltnp 2>&1 | head -15
`

// onHost reports whether the question's environment is a Linux host without
// Kubernetes.
func onHost(cat *catalog.Catalog, q *catalog.Question) bool {
	if t := cat.Tracks[q.Track]; t != nil {
		if e := t.Environments[q.Environment]; e != nil && e.Type == sandbox.EnvContainer {
			return true
		}
	}
	return false
}

// SnapshotScript picks the read-only snapshot script that fits the
// question's environment: a cluster or a Linux host.
func SnapshotScript(cat *catalog.Catalog, q *catalog.Question) string {
	if onHost(cat, q) {
		return hostSnapshot
	}
	return clusterSnapshot
}

// clip cuts s to at most n bytes, never inside a character.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "\n… (cut)"
}

// observed is what was read from the sandbox for one state of a question.
type observed struct {
	snapshot    string
	diagnostics []ai.PromptDiagnostic
	at          time.Time
}

// observations caches what was read from the sandbox, so that a follow-up
// question does not wait for the same commands again.
type observations struct {
	mu sync.Mutex
	m  map[string]observed
}

func (o *observations) get(key string, now time.Time) (observed, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	v, ok := o.m[key]
	if !ok || now.Sub(v.at) > contextTTL {
		return observed{}, false
	}
	return v, true
}

func (o *observations) put(key string, v observed) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.m == nil {
		o.m = map[string]observed{}
	}
	for k, old := range o.m {
		if v.at.Sub(old.at) > contextTTL {
			delete(o.m, k)
		}
	}
	o.m[key] = v
}

func (o *observations) drop(prefix string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for k := range o.m {
		if strings.HasPrefix(k, prefix) {
			delete(o.m, k)
		}
	}
}

// failing returns the diagnostics for the failing checks of the last result:
// at most maxDiagnostics, in the order of the checks.
func failing(q *catalog.Question, last *checker.Result) []diagnostic {
	if last == nil || last.Passed {
		return nil
	}
	failed := map[string]bool{}
	for _, c := range last.Checks {
		if !c.Passed {
			failed[c.ID] = true
		}
	}
	var out []diagnostic
	for _, c := range q.Checks {
		if !failed[c.ID] {
			continue
		}
		if d, ok := diagnosticFor(c); ok {
			if out = append(out, d); len(out) == maxDiagnostics {
				break
			}
		}
	}
	return out
}

// promptContext describes the learner's task as kubedojo knows it without
// asking the sandbox: the task, the last check result, what the learner has
// opened and written.
func (s *Service) promptContext(v *session.Session, q *catalog.Question, qs *session.QState) ai.PromptContext {
	host := v.Envs[q.Environment]
	render := func(text string) string { return s.cfg.Render(text, host) }
	pc := ai.PromptContext{Title: q.Title, Domain: q.Domain, Task: render(q.Task), Note: qs.Note, HintsTotal: len(q.Hints)}
	switch {
	case q.Type == catalog.TypeChoice:
		pc.Environment = "none: a multiple-choice question"
	case onHost(s.cfg.Catalog, q):
		pc.Environment = "a plain Linux host without Kubernetes"
	default:
		pc.Environment = "a Kubernetes cluster"
	}
	for i := 0; i < qs.HintsUsed && i < len(q.Hints); i++ {
		pc.Hints = append(pc.Hints, render(q.Hints[i]))
	}
	if qs.SolutionViewed {
		pc.SolutionViewed = true
		if q.Type != catalog.TypeChoice {
			pc.Solution = render(q.SolutionMd)
		}
	}
	if qs.Last != nil {
		pc.Checks = &ai.PromptChecks{Score: qs.Last.Score, Max: qs.Last.Max, Passed: qs.Last.Passed}
		if len(qs.Last.Checks) > 0 {
			// The result is what the learner sees: clipped and with
			// credentials redacted by the checker.
			raw, _ := json.MarshalIndent(qs.Last.Checks, "", "  ")
			pc.Checks.JSON = string(raw)
		} else if qs.Last.Passed {
			pc.Checks.JSON = "The answer was graded as correct."
		} else {
			pc.Checks.JSON = "The answer was graded as wrong."
		}
	}
	if c := q.Choice; c != nil {
		ch := &ai.PromptChoice{Multiple: c.Multiple, Revealed: v.Revealed(qs)}
		byID := map[string]*catalog.Option{}
		for _, o := range c.Options {
			byID[o.ID] = o
		}
		picked := map[string]bool{}
		for _, id := range qs.Answer {
			picked[id] = true
		}
		// The options in the order drawn for this session, as the learner
		// sees them.
		for _, id := range qs.Order {
			o := byID[id]
			if o == nil {
				continue
			}
			po := ai.PromptOption{Text: o.Text, Picked: picked[id]}
			if ch.Revealed {
				po.Correct, po.Explain = o.Correct, o.Explain
			}
			ch.Options = append(ch.Options, po)
		}
		if ch.Revealed {
			ch.Explanation = c.Explanation
		}
		pc.Choice = ch
	}
	return pc
}

// observe reads the sandbox: the snapshot and the diagnostics of the failing
// checks. It reports each stage it actually runs. Nothing here can fail a
// message: what cannot be read is left out. cached says the sandbox was not
// asked: what was read for an earlier message still holds.
func (s *Service) observe(ctx context.Context, v *session.Session, q *catalog.Question, qs *session.QState, stage func(string)) (seen observed, cached bool) {
	if qs.Status != "ready" || q.Type == catalog.TypeChoice || s.cfg.Sessions == nil {
		return observed{}, false
	}
	// The state of the sandbox is taken to hold for a short while, and
	// certainly not beyond the next check.
	key := v.ID + "/" + q.ID + "/"
	if qs.Last != nil {
		key += qs.Last.At.UTC().Format(time.RFC3339Nano)
	}
	now := s.now()
	if got, ok := s.seen.get(key, now); ok {
		return got, true
	}
	out := observed{at: now}

	stage(StageSnapshot)
	sctx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	res, err := s.cfg.Sessions.Exec(session.WithExecPurpose(sctx, session.ExecSnapshot), v.ID, q.ID, "", SnapshotScript(s.cfg.Catalog, q))
	cancel()
	if err == nil {
		out.snapshot = clip(checker.Redact(strings.TrimSpace(res.Stdout)), snapshotSize)
	}

	if plan := failing(q, qs.Last); len(plan) > 0 && ctx.Err() == nil {
		stage(StageDiagnostics)
		all, cancelAll := context.WithTimeout(ctx, diagnosticsTimeout)
		for _, d := range plan {
			if all.Err() != nil {
				break
			}
			dctx, cancel := context.WithTimeout(all, diagnosticTimeout)
			res, err := s.cfg.Sessions.Exec(session.WithExecPurpose(dctx, session.ExecDiagnostic), v.ID, q.ID, d.Role, d.Script)
			cancel()
			if err != nil {
				continue
			}
			text := strings.TrimSpace(res.Stdout)
			if e := strings.TrimSpace(res.Stderr); e != "" {
				text = strings.TrimSpace(text + "\n" + e)
			}
			if text == "" {
				continue
			}
			out.diagnostics = append(out.diagnostics, ai.PromptDiagnostic{Check: d.CheckID, Command: d.Display,
				Output: clip(checker.Redact(text), diagnosticOutputSize)})
		}
		cancelAll()
	}
	if ctx.Err() == nil {
		s.seen.put(key, out)
	}
	return out, false
}

// gather collects what the model is told about the task with this message:
// what kubedojo knows without asking, and what it reads from the sandbox.
// It is the span assistant.context; the commands that run in the sandbox
// are the spans below it. The span describes the context by its size and
// by what went into it, never by its text.
func (t *Turn) gather(ctx context.Context, stage func(string)) ai.PromptContext {
	s, began := t.s, time.Now()
	ctx, sp := span.Start(ctx, "assistant.context", t.attrs()...)
	pc := s.promptContext(t.v, t.q, t.qs)
	seen, cached := s.observe(ctx, t.v, t.q, t.qs, stage)
	pc.Snapshot, pc.Diagnostics = seen.snapshot, seen.diagnostics
	failed := 0
	if t.qs.Last != nil {
		for _, c := range t.qs.Last.Checks {
			if !c.Passed {
				failed++
			}
		}
	}
	sp.Set(span.Int("kubedojo.assistant.context.bytes", len(ai.RenderContext(pc))),
		span.Int("kubedojo.assistant.context.failing_checks", failed),
		span.Int("kubedojo.assistant.context.hints_used", len(pc.Hints)),
		span.Bool("kubedojo.assistant.context.solution_viewed", pc.SolutionViewed),
		span.Bool("kubedojo.assistant.context.cached", cached))
	// Inside the span, so the sample carries its trace.
	if s.cfg.Hooks.Context != nil {
		s.cfg.Hooks.Context(ctx, t.v.Track, cached, time.Since(began))
	}
	sp.End(nil)
	return pc
}
