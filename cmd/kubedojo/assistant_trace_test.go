package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/Mistral-valaise/kubedojo/internal/ai/aitest"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/telemetry/teltest"
)

const (
	chatKeyMarker = "sk-CHAT-KEY-MARKER-0123456789abcdef"
	chatMessage   = "why does it fail? LEARNER-MESSAGE-MARKER"
)

// modelServer is the fake model server, and it keeps the headers it was sent.
type modelServer struct {
	aitest.Fake
	mu   sync.Mutex
	seen []http.Header
}

func (m *modelServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.seen = append(m.seen, r.Header.Clone())
	m.mu.Unlock()
	m.Fake.ServeHTTP(w, r)
}

// sseEvents parses a stream of server-sent events into name and data.
func sseEvents(t *testing.T, body string) (names []string, data map[string]map[string]any) {
	t.Helper()
	data = map[string]map[string]any{}
	var name string
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
			names = append(names, name)
		case strings.HasPrefix(line, "data: "):
			var v map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &v); err != nil {
				t.Fatalf("%s: %v", line, err)
			}
			data[name] = v
		}
	}
	return names, data
}

// recorded is every string the spans hold: names, status, attributes, events.
func recorded(spans []sdktrace.ReadOnlySpan) string {
	var b strings.Builder
	for _, s := range spans {
		fmt.Fprintln(&b, s.Name(), s.Status().Description)
		for _, kv := range s.Attributes() {
			fmt.Fprintln(&b, kv.Key, kv.Value.String())
		}
		for _, e := range s.Events() {
			fmt.Fprintln(&b, e.Name)
			for _, kv := range e.Attributes {
				fmt.Fprintln(&b, kv.Key, kv.Value.String())
			}
		}
	}
	return b.String()
}

// labelSets returns the label sets of the data points of a metric, and the
// names of all metrics there are.
func (c *correlation) labelSets(t *testing.T, name string) (sets []string, names []string, raw string) {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := c.reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	dump, err := json.Marshal(rm)
	if err != nil {
		t.Fatal(err)
	}
	add := func(set attribute.Set) {
		var parts []string
		for _, kv := range set.ToSlice() {
			parts = append(parts, string(kv.Key)+"="+kv.Value.String())
		}
		sets = append(sets, strings.Join(parts, " "))
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			names = append(names, m.Name)
			if m.Name != name {
				continue
			}
			switch d := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, p := range d.DataPoints {
					add(p.Attributes)
				}
			case metricdata.Histogram[float64]:
				for _, p := range d.DataPoints {
					add(p.Attributes)
				}
			case metricdata.Histogram[int64]:
				for _, p := range d.DataPoints {
					add(p.Attributes)
				}
			}
		}
	}
	sort.Strings(sets)
	return sets, names, string(dump)
}

// One message to the assistant, through the handler chain the server serves
// and the real OpenAI-compatible adapter, against a fake model server: the
// request, the answer, the gathering of the context with the commands it
// runs, the call to the model and its HTTP request are one trace. The stream
// names that trace. The model server gets no trace context, and its key is
// nowhere. The conversation is in the trace only when that was asked for.
func TestAChatWithTheAssistantIsOneTrace(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flag    []string
		content bool
	}{
		{"content switched on", []string{"--ai-trace-content=on"}, true},
		// auto, without the bundled local stack: the traces go somewhere
		// kubedojo knows nothing about.
		{"by default", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearAIEnv(t)
			t.Setenv("KUBEDOJO_AI_API_KEY", chatKeyMarker)
			// The process propagates trace context, as telemetry.Setup has it.
			before := otel.GetTextMapPropagator()
			otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
			t.Cleanup(func() { otel.SetTextMapPropagator(before) })

			model := &modelServer{Fake: aitest.Fake{RequireKey: chatKeyMarker}}
			srv := httptest.NewServer(model)
			t.Cleanup(srv.Close)

			c := newCorrelation(t)
			flags, err := parseAI(t, append([]string{"--ai", "--ai-provider", "openai", "--ai-base-url", srv.URL + "/v1", "--ai-model", "fake"}, tc.flag...)...)
			if err != nil {
				t.Fatal(err)
			}
			parts := serverParts{catalog: c.cat, sessions: c.mgr, store: c.st, telemetry: c.tel, log: c.tel.Logger, ai: flags,
				dataDir: t.TempDir(), addr: "127.0.0.1:4400", sandboxHost: "local"}
			h := c.tel.HTTP(newAPIServer(parts).Handler())
			q := c.cat.Tracks["cka"].Questions[0]

			// A learner with a question that is prepared and not solved.
			res, _ := c.call(t, h, "POST", "/api/sessions", `{"mode":"practice","track":"cka"}`)
			var ses struct{ ID string }
			if err := json.Unmarshal(res.Body.Bytes(), &ses); err != nil || ses.ID == "" {
				t.Fatalf("create session: %d %s", res.Code, res.Body)
			}
			base := "/api/sessions/" + ses.ID + "/questions/" + q.ID
			if res, _ = c.call(t, h, "POST", base+"/prepare", ""); res.Code != 202 {
				t.Fatalf("prepare: %d %s", res.Code, res.Body)
			}
			c.ready(t, ses.ID)
			for deadline := time.Now().Add(5 * time.Second); len(c.named("question.prepare")) == 0 && time.Now().Before(deadline); {
				time.Sleep(5 * time.Millisecond)
			}
			if res, _ = c.call(t, h, "POST", base+"/check", ""); res.Code != 200 {
				t.Fatalf("check: %d %s", res.Code, res.Body)
			}

			// The message.
			body, _ := json.Marshal(map[string]string{"message": chatMessage})
			res, request := c.call(t, h, "POST", base+"/assistant", string(body))
			if res.Code != 200 || res.Header().Get("Content-Type") != "text/event-stream" {
				t.Fatalf("assistant: %d %s", res.Code, res.Body)
			}
			names, events := sseEvents(t, res.Body.String())
			if names[0] != "meta" || names[len(names)-1] != "done" {
				t.Fatalf("events: %v\n%s", names, res.Body)
			}
			traceID := request.SpanContext().TraceID()
			if got := events["meta"]["traceId"]; got != traceID.String() {
				t.Errorf("meta.traceId = %v, the request is the trace %s", got, traceID)
			}
			conversation := ses.ID + "/" + q.ID
			if events["meta"]["conversation"] != conversation {
				t.Errorf("meta.conversation = %v", events["meta"]["conversation"])
			}

			// The trace, from the top.
			spans := teltest.InTrace(c.spans.Ended(), traceID)
			t.Logf("the trace of one message (%s):\n%s", tc.name, teltest.Tree(spans, "kubedojo.session.id", "gen_ai.conversation.id",
				"gen_ai.system_instructions", "gen_ai.input.messages", "gen_ai.output.messages", "gen_ai.response.time_to_first_chunk",
				"server.port", "url.full", "network.peer.port", "network.peer.address", "user_agent.original", "http.request.body.size", "kubedojo.assistant.context.bytes",
				"client.address", "network.protocol.version", "http.response.body.size", "server.address", "url.scheme", "url.path", "http.route", "network.protocol.name"))
			if request.Name() != "POST /api/sessions/{id}/questions/{q}/assistant" {
				t.Errorf("the request span is named %q", request.Name())
			}
			below := func(parent sdktrace.ReadOnlySpan) []sdktrace.ReadOnlySpan {
				kids := c.children(parent)
				sort.SliceStable(kids, func(i, j int) bool { return kids[i].StartTime().Before(kids[j].StartTime()) })
				return kids
			}
			top := below(request)
			if len(top) != 1 || top[0].Name() != "assistant.chat" {
				t.Fatalf("below the request:\n%s", teltest.Shape(spans))
			}
			chat := top[0]
			steps := below(chat)
			if len(steps) != 2 || steps[0].Name() != "assistant.context" || steps[1].Name() != "chat fake" {
				t.Fatalf("below assistant.chat:\n%s", teltest.Shape(spans))
			}
			gather, call := steps[0], steps[1]
			execs := below(gather)
			if len(execs) == 0 || attr(execs[0], "kubedojo.exec.purpose") != session.ExecSnapshot {
				t.Fatalf("below assistant.context:\n%s", teltest.Shape(spans))
			}
			for _, e := range execs[1:] {
				if e.Name() != "sandbox.exec" || attr(e, "kubedojo.exec.purpose") != session.ExecDiagnostic {
					t.Errorf("below assistant.context: %s (%s)", e.Name(), attr(e, "kubedojo.exec.purpose"))
				}
			}
			posts := below(call)
			if len(posts) != 1 || posts[0].Name() != "POST "+strings.TrimPrefix(srv.URL, "http://") || posts[0].SpanKind() != trace.SpanKindClient || len(below(posts[0])) != 0 {
				t.Fatalf("below the chat span:\n%s", teltest.Shape(spans))
			}
			if len(spans) != 5+len(execs) {
				t.Errorf("%d spans in the trace, want the request, 4 of the assistant and %d commands:\n%s", len(spans), len(execs), teltest.Shape(spans))
			}
			for _, s := range spans {
				if s.Status().Code == codes.Error {
					t.Errorf("%s is an error: %s", s.Name(), s.Status().Description)
				}
			}

			// What the spans say.
			for _, s := range []sdktrace.ReadOnlySpan{request, chat, gather} {
				for key, want := range map[string]string{"kubedojo.session.id": ses.ID, "kubedojo.track": "cka", "kubedojo.mode": "practice", "kubedojo.question": q.ID} {
					if attr(s, key) != want {
						t.Errorf("%s: %s = %q, want %q", s.Name(), key, attr(s, key), want)
					}
				}
			}
			if attr(chat, "gen_ai.conversation.id") != conversation || attr(chat, "kubedojo.assistant.result") != "ok" {
				t.Errorf("assistant.chat: %v", chat.Attributes())
			}
			ca := teltest.Attrs(call)
			if call.SpanKind() != trace.SpanKindClient || ca["gen_ai.operation.name"].AsString() != "chat" || ca["gen_ai.provider.name"].AsString() != "openai_compatible" ||
				ca["gen_ai.request.model"].AsString() != "fake" || ca["gen_ai.response.model"].AsString() != "fake" || ca["gen_ai.conversation.id"].AsString() != conversation ||
				ca["gen_ai.response.id"].AsString() != "chatcmpl-fake" || ca["gen_ai.usage.input_tokens"].AsInt64() == 0 || ca["gen_ai.usage.output_tokens"].AsInt64() == 0 ||
				ca["gen_ai.response.time_to_first_chunk"].AsFloat64() <= 0 || ca["server.address"].AsString() != "127.0.0.1" {
				t.Errorf("the chat span: %s", teltest.Tree([]sdktrace.ReadOnlySpan{call}, "gen_ai.input.messages", "gen_ai.system_instructions", "gen_ai.output.messages"))
			}
			if done := events["done"]["usage"].(map[string]any); int64(done["inputTokens"].(float64)) != ca["gen_ai.usage.input_tokens"].AsInt64() {
				t.Errorf("the stream says %v input tokens, the span %d", done["inputTokens"], ca["gen_ai.usage.input_tokens"].AsInt64())
			}

			// The conversation: only if it was asked for, and then only on the chat span.
			all := recorded(c.spans.Ended())
			in, out := ca["gen_ai.input.messages"].String(), ca["gen_ai.output.messages"].String()
			if tc.content {
				if !strings.Contains(in, "LEARNER-MESSAGE-MARKER") || !strings.Contains(in, "<context>") || !strings.Contains(out, "Fake model") ||
					!strings.Contains(ca["gen_ai.system_instructions"].String(), "You are the assistant inside kubedojo") {
					t.Errorf("the conversation is not on the chat span:\n%s\n%s", in, out)
				}
				rest := recorded(append(append([]sdktrace.ReadOnlySpan{request, chat, gather}, execs...), posts...))
				if strings.Contains(rest, "LEARNER-MESSAGE-MARKER") || strings.Contains(rest, "Fake model") {
					t.Errorf("text of the conversation is on a span that is not the chat span:\n%s", rest)
				}
			} else {
				for _, k := range []string{"gen_ai.system_instructions", "gen_ai.input.messages", "gen_ai.output.messages"} {
					if _, has := ca[k]; has {
						t.Errorf("%s exists although nobody asked for content", k)
					}
				}
				for _, m := range []string{"LEARNER-MESSAGE-MARKER", "Fake model", "You are the assistant inside kubedojo", "<context>"} {
					if strings.Contains(all, m) {
						t.Errorf("%q is in a trace although nobody asked for content", m)
					}
				}
			}

			// The model server: one request, with the key and without our trace.
			model.mu.Lock()
			seen := append([]http.Header(nil), model.seen...)
			model.mu.Unlock()
			if len(seen) != 1 || seen[0].Get("Authorization") != "Bearer "+chatKeyMarker {
				t.Fatalf("requests to the model server: %v", seen)
			}
			for _, name := range []string{"Traceparent", "Tracestate", "Baggage"} {
				if v := seen[0].Get(name); v != "" {
					t.Errorf("the model server was sent %s: %s", name, v)
				}
			}

			// A message that is refused before an answer starts is counted too.
			long, _ := json.Marshal(map[string]string{"message": strings.Repeat("a", 4001)})
			if res, _ = c.call(t, h, "POST", base+"/assistant", string(long)); res.Code != 400 {
				t.Fatalf("a message that is too long: %d %s", res.Code, res.Body)
			}

			// Metrics: the samples of the message carry its trace, their labels
			// are the fixed ones, and the HTTP client adds no metrics of its own.
			for _, name := range []string{"gen_ai.client.inference.duration", "gen_ai.client.inference.time_to_first_chunk", "kubedojo.assistant.context.duration",
				"gen_ai.client.inference.usage.input_tokens", "gen_ai.client.inference.usage.output_tokens"} {
				traces, keys := c.exemplars(t, name)
				if len(traces) != 1 || traces[0] != traceID {
					t.Errorf("exemplars of %s: %v, want the trace %s", name, traces, traceID)
				}
				for k := range keys {
					if strings.Contains(k, "session") || strings.Contains(k, "conversation") || strings.Contains(k, "question") {
						t.Errorf("%s is labelled with %q", name, k)
					}
				}
			}
			if traces, _ := c.exemplars(t, "kubedojo.assistant.requests"); len(traces) != 1 || traces[0] != traceID {
				t.Errorf("exemplars of kubedojo.assistant.requests: %v", traces)
			}
			sets, metricNames, dump := c.labelSets(t, "kubedojo.assistant.requests")
			want := []string{
				"mode=practice phase=admission provider=openai_compatible result=too_long track=cka",
				"mode=practice phase=answer provider=openai_compatible result=ok track=cka",
			}
			if strings.Join(sets, "\n") != strings.Join(want, "\n") {
				t.Errorf("kubedojo.assistant.requests:\n%s\nwant:\n%s", strings.Join(sets, "\n"), strings.Join(want, "\n"))
			}
			for _, n := range metricNames {
				if strings.HasPrefix(n, "http.client") {
					t.Errorf("the metric %s is not in the catalog", n)
				}
			}

			// The key: in no span, no metric and no log line.
			var lines strings.Builder
			for _, l := range c.logs.all() {
				fmt.Fprintln(&lines, l.msg, l.attrs)
			}
			c.stderr.mu.Lock()
			lines.Write(c.stderr.buf.Bytes())
			c.stderr.mu.Unlock()
			for where, text := range map[string]string{"a span": all, "a metric": dump, "a log line": lines.String()} {
				if strings.Contains(text, "CHAT-KEY-MARKER") {
					t.Errorf("the key is in %s", where)
				}
				if where != "a span" && (strings.Contains(text, "LEARNER-MESSAGE-MARKER") || strings.Contains(text, "Fake model")) {
					t.Errorf("text of the conversation is in %s", where)
				}
			}
		})
	}
}
