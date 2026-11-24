package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/assistant"
)

// Render fills the per-session placeholders of content text ({{host}} and
// the names of the workers), as the API does for everything it shows. The
// assistant uses it for the task, the hints and the solution it tells the
// model about.
func Render(text, host string) string { return render(text, host) }

// registerAssistant registers the endpoints of the assistant.
func (s *Server) registerAssistant(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sessions/{id}/questions/{q}/assistant", s.assistantHistory)
	mux.HandleFunc("DELETE /api/sessions/{id}/questions/{q}/assistant", s.assistantClear)
	mux.HandleFunc("POST /api/sessions/{id}/questions/{q}/assistant", s.assistantChat)
	// Deprecated: the endpoint of the one-shot tutor, kept for 0.5.x.
	mux.HandleFunc("POST /api/sessions/{id}/questions/{q}/tutor", s.tutor)
}

// writeCoded answers an error that has a machine-readable code next to its
// text. extra adds fields.
func writeCoded(w http.ResponseWriter, status int, code, msg string, extra map[string]any) {
	body := map[string]any{"error": msg}
	if code != "" {
		body["code"] = code
	}
	for k, v := range extra {
		body[k] = v
	}
	writeJSON(w, status, body)
}

// refuse answers a request the assistant did not admit.
func (s *Server) refuse(w http.ResponseWriter, r *http.Request, err error) {
	var ref *assistant.Refusal
	if !errors.As(err, &ref) {
		s.Log.ErrorContext(r.Context(), "assistant", "route", r.Pattern, "err", err)
		writeErr(w, 500, "The assistant failed. The server log has the details.")
		return
	}
	if ref.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(ref.RetryAfter.Seconds())+1))
	}
	writeCoded(w, ref.Status, ref.Code, ref.Message, nil)
}

// jsonBody reports whether the request says its body is JSON. The endpoints
// that cost something (a model call, a change of the settings) insist on it:
// a web page of another origin cannot send that content type without the
// browser asking this server first, and this server never says yes.
func jsonBody(w http.ResponseWriter, r *http.Request) bool {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err == nil && mt == "application/json" {
		return true
	}
	writeErr(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
	return false
}

type messageView struct {
	ID      int64     `json:"id"`
	Role    string    `json:"role"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
	Partial bool      `json:"partial"`
}

func (s *Server) assistantHistory(w http.ResponseWriter, r *http.Request) {
	v, q, _ := s.question(w, r)
	if v == nil {
		return
	}
	out := []messageView{}
	if s.Assistant != nil {
		rows, err := s.Assistant.History(v, q)
		if err != nil {
			s.refuse(w, r, err)
			return
		}
		for _, m := range rows {
			out = append(out, messageView{ID: m.ID, Role: m.Role, Content: m.Content, At: m.At.UTC(), Partial: m.Partial})
		}
	}
	writeJSON(w, 200, map[string]any{"messages": out,
		"limits": map[string]int{"turns": assistant.MaxTurns, "messageChars": assistant.MaxMessageChars}})
}

func (s *Server) assistantClear(w http.ResponseWriter, r *http.Request) {
	v, q, _ := s.question(w, r)
	if v == nil {
		return
	}
	if s.Assistant != nil {
		if err := s.Assistant.Clear(v, q); err != nil {
			s.refuse(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func notConfigured() *assistant.Refusal {
	return &assistant.Refusal{Status: 409, Code: assistant.CodeNotConfigured, Message: "The assistant is not available. The assistant is not configured."}
}

func (s *Server) assistantChat(w http.ResponseWriter, r *http.Request) {
	v, q, qs := s.question(w, r)
	if v == nil {
		return
	}
	if !jsonBody(w, r) {
		return
	}
	var req struct {
		Message string `json:"message"`
		RetryOf int64  `json:"retryOf"`
	}
	if !s.decode(w, r, &req, 8<<10) {
		return
	}
	if s.Assistant == nil {
		s.refuse(w, r, notConfigured())
		return
	}
	turn, err := s.Assistant.Start(s.owner(r), v, q, qs, assistant.Input{Message: req.Message, RetryOf: req.RetryOf})
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	// From here on the answer is a stream: what goes wrong is an event.
	stream := newEventStream(w, s.Assistant.PingInterval())
	defer stream.close()
	turn.Run(r.Context(), stream.send)
}

// eventStream writes server-sent events. Every event is flushed at once, and
// while nothing else is sent a comment line keeps proxies from closing the
// connection.
type eventStream struct {
	w    http.ResponseWriter
	rc   *http.ResponseController
	mu   sync.Mutex
	err  error
	done chan struct{}
	idle sync.WaitGroup
}

// writeTimeout is how long one event may take to reach the client.
const writeTimeout = 30 * time.Second

func newEventStream(w http.ResponseWriter, ping time.Duration) *eventStream {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // nginx: do not buffer the stream
	w.WriteHeader(http.StatusOK)
	e := &eventStream{w: w, rc: http.NewResponseController(w), done: make(chan struct{})}
	e.write("")
	e.idle.Add(1)
	go func() {
		defer e.idle.Done()
		t := time.NewTicker(ping)
		defer t.Stop()
		for {
			select {
			case <-e.done:
				return
			case <-t.C:
				e.write(": ping\n\n")
			}
		}
	}()
	return e
}

// write sends text and flushes. A server-wide write timeout would cut a long
// answer off, so the deadline is moved along with every write.
func (e *eventStream) write(text string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err != nil {
		return e.err
	}
	// Not every ResponseWriter has deadlines; one without them has no
	// timeout to extend either.
	_ = e.rc.SetWriteDeadline(time.Now().Add(writeTimeout))
	if text != "" {
		if _, err := e.w.Write([]byte(text)); err != nil {
			e.err = err
			return err
		}
	}
	if err := e.rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		e.err = err
		return err
	}
	return nil
}

func (e *eventStream) close() {
	close(e.done)
	e.idle.Wait()
	// The connection may serve another request. A server without a write
	// timeout does not reset the deadline for it, so it is taken back here.
	_ = e.rc.SetWriteDeadline(time.Time{})
}

func (e *eventStream) event(name string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return e.write("event: " + name + "\ndata: " + string(raw) + "\n\n")
}

type usageView struct {
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int64 `json:"cacheWriteTokens,omitempty"`
}

// send writes one event of the assistant in the wire format of the API.
func (e *eventStream) send(ev assistant.Event) error {
	switch ev := ev.(type) {
	case assistant.Meta:
		body := map[string]any{"conversation": ev.Conversation, "provider": ev.Provider, "model": ev.Model,
			"turn": ev.Turn, "userMessageId": ev.UserMessageID}
		if ev.TraceID != "" {
			body["traceId"] = ev.TraceID
		}
		return e.event("meta", body)
	case assistant.Context:
		return e.event("context", map[string]string{"stage": ev.Stage})
	case assistant.Delta:
		return e.event("delta", map[string]string{"text": ev.Text})
	case assistant.Done:
		return e.event("done", map[string]any{"messageId": ev.MessageID, "finishReason": ev.FinishReason, "model": ev.Model,
			"usage":      usageView{ev.Usage.InputTokens, ev.Usage.OutputTokens, ev.Usage.CacheReadTokens, ev.Usage.CacheWriteTokens},
			"durationMs": ev.Duration.Milliseconds()})
	case assistant.Error:
		body := map[string]any{"code": string(ev.Code), "message": ev.Message, "retryable": ev.Retryable}
		if ev.MessageID != 0 {
			body["messageId"] = ev.MessageID
		}
		return e.event("error", body)
	}
	return fmt.Errorf("unknown event %T", ev)
}

// tutor is the endpoint of the one-shot tutor that the assistant replaces.
// It answers exactly as before, {"markdown": "..."}, by collecting the
// stream of the assistant.
//
// Deprecated: clients use POST .../assistant. This endpoint goes with 0.6.
func (s *Server) tutor(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Link", `<`+strings.TrimSuffix(r.URL.Path, "/tutor")+`/assistant>; rel="successor-version"`)
	v, q, qs := s.question(w, r)
	if v == nil {
		return
	}
	if s.Assistant == nil || !s.Assistant.Status().Configured {
		writeErr(w, 404, "The AI tutor is not configured. Set ANTHROPIC_API_KEY to enable it.")
		return
	}
	var req struct {
		Ask string `json:"ask"`
	}
	if !s.decode(w, r, &req, 8<<10) {
		return
	}
	var text strings.Builder
	var failed *assistant.Error
	err := s.Assistant.Chat(r.Context(), s.owner(r), v, q, qs, assistant.Input{Message: req.Ask}, func(ev assistant.Event) error {
		switch ev := ev.(type) {
		case assistant.Delta:
			text.WriteString(ev.Text)
		case assistant.Error:
			failed = &ev
		}
		return nil
	})
	var ref *assistant.Refusal
	switch {
	case errors.As(err, &ref) && ref.Code == assistant.CodeExamRunning:
		writeErr(w, 409, "The tutor is available after the exam.")
	case err != nil:
		s.refuse(w, r, err)
	case failed != nil:
		writeErr(w, 502, "The tutor could not answer: "+failed.Message)
	default:
		writeJSON(w, 200, map[string]string{"markdown": text.String()})
	}
}
