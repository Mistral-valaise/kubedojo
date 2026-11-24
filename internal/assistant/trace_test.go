package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
	"github.com/Mistral-valaise/kubedojo/internal/ai/settings"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/span"
	"github.com/Mistral-valaise/kubedojo/internal/telemetry/teltest"
)

// counted is what the hooks were called with.
type counted struct {
	mu       sync.Mutex
	requests []string // provider phase result track mode
	contexts []string // track cached
	traced   []bool   // whether the hook ran inside a recording span
}

func (c *counted) hooks() Hooks {
	return Hooks{
		Request: func(ctx context.Context, provider, phase, result, track, mode string) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.requests = append(c.requests, strings.Join([]string{provider, phase, result, track, mode}, " "))
			c.traced = append(c.traced, span.TraceID(ctx) != "")
		},
		Context: func(ctx context.Context, track string, cached bool, d time.Duration) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.contexts = append(c.contexts, fmt.Sprint(track, " ", cached))
			c.traced = append(c.traced, span.TraceID(ctx) != "")
		},
	}
}

// request starts the span of an API request, as telemetry.HTTP does: the
// context carries the span and the tracer.
func request(rec *teltest.Recorder) (context.Context, func() sdktrace.ReadOnlySpan) {
	ctx, sp := rec.Tracer().Start(span.With(context.Background(), rec), "POST /api/sessions/{id}/questions/{q}/assistant")
	return ctx, func() sdktrace.ReadOnlySpan {
		sp.End()
		all := rec.Named("POST /api/sessions/{id}/questions/{q}/assistant")
		return all[len(all)-1]
	}
}

func only(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	var found []sdktrace.ReadOnlySpan
	for _, s := range spans {
		if s.Name() == name {
			found = append(found, s)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d spans named %s in:\n%s", len(found), name, teltest.Tree(spans))
	}
	return found[0]
}

// An answer is a span below its request, and gathering the context a span
// below that. They say how much went into the answer and never what.
func TestAnAnswerIsATrace(t *testing.T) {
	f := newFixture(t, settings.Inputs{})
	rec, calls := teltest.New(), &counted{}
	f.svc.cfg.Hooks = calls.hooks()
	q := f.question("k", "q")
	v := practice("s1", "k")
	f.sandbox.answers = map[string]string{"## nodes": "SNAPSHOT-MARKER"}
	qs := &session.QState{ID: "q", Status: "ready", Note: "NOTE-MARKER", HintsUsed: 2, SolutionViewed: true, Last: failed(q, "image", "ready")}

	ctx, end := request(rec)
	events, err := f.collect(ctx, v, q, qs, Input{Message: "why does it fail? MESSAGE-MARKER"})
	if err != nil {
		t.Fatal(err)
	}
	req := end()
	traceID := req.SpanContext().TraceID()

	// The stream names the trace.
	if meta := events[0].(Meta); meta.TraceID != traceID.String() {
		t.Errorf("meta.TraceID = %q, the request is in %s", meta.TraceID, traceID)
	}

	spans := teltest.InTrace(rec.Ended(), traceID)
	if got, want := teltest.Shape(spans), "POST /api/sessions/{id}/questions/{q}/assistant\n   assistant.chat\n      assistant.context\n"; got != want {
		t.Fatalf("the trace:\n%s\nwant:\n%s", got, want)
	}
	chat := only(t, spans, "assistant.chat")
	a := teltest.Attrs(chat)
	for k, want := range map[string]string{
		"kubedojo.session.id": "s1", "kubedojo.track": "k", "kubedojo.mode": "practice", "kubedojo.question": "q",
		"gen_ai.conversation.id": "s1/q", "kubedojo.assistant.result": "ok",
	} {
		if a[k].AsString() != want {
			t.Errorf("assistant.chat: %s = %q, want %q", k, a[k].AsString(), want)
		}
	}
	if a["kubedojo.assistant.turn"].AsInt64() != 1 || a["kubedojo.assistant.history.messages"].AsInt64() != 0 {
		t.Errorf("assistant.chat: turn %d, history %d", a["kubedojo.assistant.turn"].AsInt64(), a["kubedojo.assistant.history.messages"].AsInt64())
	}
	if chat.Status().Code != codes.Unset {
		t.Errorf("assistant.chat has the status %v", chat.Status())
	}

	gather := only(t, spans, "assistant.context")
	g := teltest.Attrs(gather)
	sent := f.provider.Requests()[0].Messages[0].Content
	block := sent[:strings.Index(sent, "</context>")+len("</context>")]
	if g["kubedojo.assistant.context.bytes"].AsInt64() != int64(len(block)) || len(block) < 500 {
		t.Errorf("kubedojo.assistant.context.bytes = %d, the block sent has %d", g["kubedojo.assistant.context.bytes"].AsInt64(), len(block))
	}
	if g["kubedojo.assistant.context.failing_checks"].AsInt64() != 3 || g["kubedojo.assistant.context.hints_used"].AsInt64() != 2 ||
		!g["kubedojo.assistant.context.solution_viewed"].AsBool() || g["kubedojo.assistant.context.cached"].AsBool() {
		t.Errorf("assistant.context: %s", teltest.Tree([]sdktrace.ReadOnlySpan{gather}))
	}
	if g["kubedojo.session.id"].AsString() != "s1" || g["kubedojo.question"].AsString() != "q" {
		t.Errorf("assistant.context does not name its session and question")
	}

	// A follow-up within the half minute: the context comes from the cache,
	// and two earlier messages go along.
	ctx, end = request(rec)
	if _, err := f.collect(ctx, v, q, qs, Input{Message: "and now?"}); err != nil {
		t.Fatal(err)
	}
	second := teltest.InTrace(rec.Ended(), end().SpanContext().TraceID())
	if !teltest.Attrs(only(t, second, "assistant.context"))["kubedojo.assistant.context.cached"].AsBool() {
		t.Error("the second context is not marked as cached")
	}
	if a := teltest.Attrs(only(t, second, "assistant.chat")); a["kubedojo.assistant.turn"].AsInt64() != 2 || a["kubedojo.assistant.history.messages"].AsInt64() != 2 {
		t.Errorf("second assistant.chat: turn %d, history %d", a["kubedojo.assistant.turn"].AsInt64(), a["kubedojo.assistant.history.messages"].AsInt64())
	}

	// Nothing anybody wrote or the sandbox printed is on these spans.
	all := teltest.Tree(rec.Ended())
	for _, m := range []string{"MESSAGE-MARKER", "NOTE-MARKER", "SNAPSHOT-MARKER", "and now?", "Fake model", "Scale the web"} {
		if strings.Contains(all, m) {
			t.Errorf("%q is in the trace:\n%s", m, all)
		}
	}

	// The counters were told, inside the spans.
	if got := strings.Join(calls.requests, "; "); got != "fake answer ok k practice; fake answer ok k practice" {
		t.Errorf("requests counted: %s", got)
	}
	if got := strings.Join(calls.contexts, "; "); got != "k false; k true" {
		t.Errorf("contexts counted: %s", got)
	}
	for i, in := range calls.traced {
		if !in {
			t.Errorf("hook call %d ran outside a recording span: no exemplar", i)
		}
	}
}

// Without a tracer the stream names no trace, and nothing else changes.
func TestWithoutTelemetryTheStreamNamesNoTrace(t *testing.T) {
	f := newFixture(t, settings.Inputs{})
	events, err := f.collect(context.Background(), practice("s1", "k"), f.question("k", "q"), &session.QState{ID: "q", Status: "pending"}, Input{Message: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if meta := events[0].(Meta); meta.TraceID != "" {
		t.Errorf("meta.TraceID = %q", meta.TraceID)
	}
}

// How an answer ended is on its span and in the counter. Only a fault is an
// error: a model that declines has answered, and a learner who leaves is no
// fault of the server.
func TestTheOutcomeOfAnAnswer(t *testing.T) {
	for _, c := range []struct {
		name    string
		err     error
		leave   bool
		result  string
		status  codes.Code
		refused string
	}{
		{name: "answered", result: "ok", status: codes.Unset},
		{name: "the key is rejected", err: &ai.Error{Kind: ai.ErrAuth, Status: 401, Message: "The provider rejected the API key.", Err: errors.New("CAUSE-MARKER")}, result: "auth", status: codes.Error},
		{name: "the provider limits", err: &ai.Error{Kind: ai.ErrRateLimited, Status: 429, Message: "Try again."}, result: "rate_limited", status: codes.Error},
		{name: "not an ai.Error", err: errors.New("CAUSE-MARKER"), result: "upstream", status: codes.Error},
		{name: "the model declines", err: ai.Refused(), result: "refused", status: codes.Unset},
		{name: "the learner leaves", leave: true, result: "canceled", status: codes.Unset, refused: "cancelled"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t, settings.Inputs{})
			rec, calls := teltest.New(), &counted{}
			f.svc.cfg.Hooks = calls.hooks()
			f.provider.Deltas = []string{"one ", "two"}
			f.provider.Err, f.provider.ErrAfter = c.err, 1
			ctx, end := request(rec)
			err := f.svc.Chat(ctx, "", practice("s1", "z"), f.question("z", "c1"), &session.QState{ID: "c1"}, Input{Message: "x"}, func(e Event) error {
				if _, ok := e.(Delta); ok && c.leave {
					return errors.New("broken pipe")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			end()
			chat := only(t, rec.Ended(), "assistant.chat")
			a := teltest.Attrs(chat)
			if a["kubedojo.assistant.result"].AsString() != c.result || chat.Status().Code != c.status || a["kubedojo.refused"].AsString() != c.refused {
				t.Errorf("assistant.chat: result %q, status %v, refused %q", a["kubedojo.assistant.result"].AsString(), chat.Status(), a["kubedojo.refused"].AsString())
			}
			kind, has := a["error.type"]
			if c.status == codes.Error && kind.AsString() != c.result || c.status != codes.Error && has {
				t.Errorf("error.type = %q", kind.AsString())
			}
			// The cause of a failure can quote the upstream: it is in the log,
			// not in the trace.
			if all := teltest.Tree(rec.Ended()); strings.Contains(all, "CAUSE-MARKER") {
				t.Errorf("the cause is in the trace:\n%s", all)
			}
			if got := strings.Join(calls.requests, "; "); got != "fake answer "+c.result+" z practice" {
				t.Errorf("requests counted: %s", got)
			}
		})
	}
}

// A message kubedojo refuses before an answer starts is counted with the
// code of the refusal, apart from the answers that failed.
func TestRefusedMessagesAreCounted(t *testing.T) {
	f := newFixture(t, settings.Inputs{})
	calls := &counted{}
	f.svc.cfg.Hooks = calls.hooks()
	q := f.question("k", "q")
	qs := &session.QState{ID: "q", Status: "pending"}
	nobody := func(Event) error { return nil }

	// Too long, and an exam that is running.
	_ = f.svc.Chat(context.Background(), "", practice("s1", "k"), q, qs, Input{Message: strings.Repeat("a", MaxMessageChars+1)}, nobody)
	exam := &session.Session{ID: "e1", Mode: session.ModeExam, Track: "k", Status: session.StatusRunning}
	_ = f.svc.Chat(context.Background(), "", exam, q, qs, Input{Message: "help"}, nobody)
	// Busy: the question is being answered. Then every stream is taken.
	var turns []*Turn
	for i := range MaxStreams {
		turn, err := f.svc.Start("", practice(fmt.Sprint("s", i), "k"), q, qs, Input{Message: "x"})
		if err != nil {
			t.Fatal(err)
		}
		turns = append(turns, turn)
	}
	_, _ = f.svc.Start("", practice("s0", "k"), q, qs, Input{Message: "again"})
	_, _ = f.svc.Start("", practice("other", "k"), q, qs, Input{Message: "x"})
	for _, turn := range turns {
		turn.Close()
	}
	// A retry of a message that does not exist.
	_, _ = f.svc.Start("", practice("s1", "k"), q, qs, Input{RetryOf: 4711})

	want := []string{
		"fake admission too_long k practice",
		"fake admission exam_running k exam",
		"fake admission busy k practice",
		"fake admission rate_limited k practice",
		"fake admission no_such_message k practice",
	}
	if got := strings.Join(calls.requests, "; "); got != strings.Join(want, "; ") {
		t.Errorf("counted:\n  %s\nwant:\n  %s", strings.ReplaceAll(got, "; ", "\n  "), strings.Join(want, "\n  "))
	}

	// Switched off: there is no provider to name.
	off := newFixture(t, settings.Inputs{Flags: map[string]string{}})
	calls = &counted{}
	off.svc.cfg.Hooks = calls.hooks()
	_ = off.svc.Chat(context.Background(), "", practice("s1", "k"), q, qs, Input{Message: "x"}, nobody)
	if got := strings.Join(calls.requests, "; "); got != "none admission not_configured k practice" {
		t.Errorf("counted while switched off: %s", got)
	}
}
