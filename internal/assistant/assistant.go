// Package assistant is the AI assistant of kubedojo: a conversation about
// one question, between a learner and a model.
//
// The service owns everything between the HTTP handler and the provider: the
// limits, the stored conversation, the context it gathers for each message,
// the prompt, and the stream of events the handler passes on. The model gets
// no tools. What it knows about the learner's environment, kubedojo read for
// it with commands kubedojo chose.
package assistant

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
	"github.com/Mistral-valaise/kubedojo/internal/ai/providers"
	"github.com/Mistral-valaise/kubedojo/internal/ai/settings"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/ratelimit"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/span"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

// Limits of a conversation.
const (
	// MaxTurns is how many earlier exchanges (a message and its answer) are
	// sent to the model with a new message. Older ones stay stored.
	MaxTurns = 20
	// MaxMessageChars is the longest message a learner can send.
	MaxMessageChars = 4000
	// RequestsPerMinute is how many messages one user may send per minute.
	RequestsPerMinute = 10
	// MaxStreams is how many answers the server streams at once.
	MaxStreams = 4
	// DefaultPingInterval is how often an idle stream sends a comment line.
	DefaultPingInterval = 15 * time.Second
)

// Codes of a request that is refused before an answer starts.
const (
	CodeExamRunning   = "exam_running"
	CodeBusy          = "busy"
	CodeNotConfigured = "not_configured"
	CodeRateLimited   = "rate_limited"
	CodeTooLong       = "too_long"
	CodeNoMessage     = "no_such_message"
	CodeBadRetry      = "bad_retry"
	CodeHosted        = "hosted"
)

// Refusal is why a request was refused before an answer started. Status is
// the HTTP status that says so.
type Refusal struct {
	Status     int
	Code       string
	Message    string
	RetryAfter time.Duration
}

func (r *Refusal) Error() string { return r.Message }

// Sessions is what the service needs of the session manager: running a
// read-only script on a machine of a question's environment.
type Sessions interface {
	Exec(ctx context.Context, id, qid, role, script string) (sandbox.ExecResult, error)
}

// Config is what the service is built from.
type Config struct {
	Holder   *ai.Holder
	Settings *settings.Resolver
	// Factory builds the provider of a settings test.
	Factory  providers.Factory
	Store    *store.Store
	Sessions Sessions
	Catalog  *catalog.Catalog
	Log      *slog.Logger
	// Render fills the per-session placeholders of content text, as the API
	// does for the task it shows.
	Render func(text, host string) string
	// LocalStack says the bundled observability stack is running, which is
	// what --ai-trace-content=auto looks at.
	LocalStack bool
	// PingInterval overrides DefaultPingInterval.
	PingInterval time.Duration
	// Hooks connect the assistant to the metrics of the server.
	Hooks Hooks
}

// What the request counter is told about a message.
const (
	// PhaseAdmission is before an answer starts: kubedojo may refuse.
	PhaseAdmission = "admission"
	// PhaseAnswer is the call to the model.
	PhaseAnswer = "answer"
	// ResultOK is the result of a message that was answered.
	ResultOK = "ok"
)

// Hooks are called for what the assistant does, so that it can be counted.
// A hook that is nil is not called. Every argument is one of a small fixed
// set of words (or the name of the configured provider): the hooks label
// metrics with them. Nothing a learner wrote is ever passed.
type Hooks struct {
	// Request is called once for every message. phase says how far it got:
	// with PhaseAdmission it was refused before an answer started and
	// result is the code of the refusal (CodeRateLimited, CodeBusy, ...);
	// with PhaseAnswer the model was asked and result is ResultOK or the
	// ai.ErrorKind of the failure. provider is "none" while there is none.
	Request func(ctx context.Context, provider, phase, result, track, mode string)
	// Context is called when the context of a message was gathered. cached
	// says the sandbox was not asked again.
	Context func(ctx context.Context, track string, cached bool, d time.Duration)
}

func (s *Service) count(ctx context.Context, provider, phase, result string, v *session.Session) {
	if s.cfg.Hooks.Request != nil {
		s.cfg.Hooks.Request(ctx, provider, phase, result, v.Track, v.Mode)
	}
}

// Service is the assistant.
type Service struct {
	cfg     Config
	limiter *ratelimit.Limiter
	gate    *ratelimit.Gate
	seen    observations
	now     func() time.Time

	// change serialises changes of the settings.
	change sync.Mutex

	purge struct {
		sync.Mutex
		at time.Time
	}
}

// New returns the service. It also removes the conversations of sessions
// that no longer exist.
func New(cfg Config) *Service {
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.Render == nil {
		cfg.Render = func(text, _ string) string { return text }
	}
	if cfg.PingInterval <= 0 {
		cfg.PingInterval = DefaultPingInterval
	}
	s := &Service{cfg: cfg, limiter: ratelimit.NewLimiter(RequestsPerMinute, time.Minute), gate: ratelimit.NewGate(MaxStreams), now: time.Now}
	s.purgeOld()
	return s
}

// PingInterval is how often an idle stream should send a sign of life.
func (s *Service) PingInterval() time.Duration { return s.cfg.PingInterval }

// purgeOld removes the conversations of destroyed sessions, at most once an
// hour. The session manager has no hook for "a session was destroyed", so
// this runs when the service starts and then with the next message after an
// hour has passed.
func (s *Service) purgeOld() {
	s.purge.Lock()
	defer s.purge.Unlock()
	if s.cfg.Store == nil || s.now().Sub(s.purge.at) < time.Hour {
		return
	}
	s.purge.at = s.now()
	if n, err := s.cfg.Store.PurgeAssistantMessages(); err != nil {
		s.cfg.Log.Warn("assistant: old conversations were not removed", "err", err)
	} else if n > 0 {
		s.cfg.Log.Info("assistant: removed the conversations of ended sessions", "messages", n)
	}
}

// Status is the state of the assistant in a few words, for /api/config.
type Status struct {
	Enabled    bool   `json:"enabled"`
	Configured bool   `json:"configured"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Writable   bool   `json:"writable"`
}

// Status reports whether the assistant can be used and with what. Provider
// is the name of the provider in use ("anthropic", "ollama", "openai", ...),
// or the configured provider id while there is none.
func (s *Service) Status() Status {
	p, eff := s.cfg.Holder.Current()
	st := Status{Enabled: eff.Enabled.Value, Configured: p != nil, Provider: eff.Provider.Value, Model: eff.Model.Value, Writable: !eff.Hosted}
	if p != nil {
		st.Provider, st.Model = p.Name(), p.Model()
	}
	return st
}

// Event is one event of an answer: Meta, Context, Delta, Done or Error.
type Event interface{ event() }

// Meta opens the stream: which conversation, which model.
type Meta struct {
	Conversation string
	Provider     string
	Model        string
	// TraceID names the trace of this answer when it is being recorded, so
	// that the UI can show it next to an error. Empty otherwise.
	TraceID string
	// Turn counts the learner's messages of this conversation, from 1.
	Turn int
	// UserMessageID is the stored message that is being answered.
	UserMessageID int64
}

// Context says kubedojo is gathering context: one of the Stage constants.
type Context struct{ Stage string }

// Delta is a piece of the answer.
type Delta struct{ Text string }

// Done closes a complete answer.
type Done struct {
	MessageID    int64
	FinishReason string
	Model        string
	Usage        ai.Usage
	Duration     time.Duration
}

// Error closes an answer that failed. MessageID is set when the part that
// did arrive was stored (as a partial answer).
type Error struct {
	Code      ai.ErrorKind
	Message   string
	Retryable bool
	MessageID int64
}

func (Meta) event()    {}
func (Context) event() {}
func (Delta) event()   {}
func (Done) event()    {}
func (Error) event()   {}

// Sink receives the events of an answer, in order. An error means nobody is
// listening any more; the answer is then cancelled.
type Sink func(Event) error

// Input is one request of the learner.
type Input struct {
	// Message is what the learner wrote. Empty asks the default question.
	Message string
	// RetryOf, when set, is the id of the assistant's last answer: it is
	// deleted and the message before it is answered again.
	RetryOf int64
}

// Turn is a request that was admitted: it holds its place among the running
// streams until Run returns or Close is called.
type Turn struct {
	s        *Service
	v        *session.Session
	q        *catalog.Question
	qs       *session.QState
	provider ai.Provider
	prior    []store.AssistantMessage
	message  string
	userID   int64
	turn     int
	release  func()
}

// Close gives up a turn that will not be run.
func (t *Turn) Close() { t.release() }

func ownerKey(owner string) string {
	if owner == "" {
		return "local"
	}
	return "user:" + owner
}

func conversation(v *session.Session, q *catalog.Question) string { return v.ID + "/" + q.ID }

// examRunning reports whether the session is an exam that is not over: the
// assistant is not available then.
func examRunning(v *session.Session) bool {
	return v.Mode == session.ModeExam && v.Status != session.StatusFinished
}

func (s *Service) allow(owner string) *Refusal {
	if ok, wait := s.limiter.Allow(ownerKey(owner)); !ok {
		return &Refusal{Status: 429, Code: CodeRateLimited, RetryAfter: wait,
			Message: fmt.Sprintf("Too many messages. Try again in %d seconds.", int(wait.Seconds())+1)}
	}
	return nil
}

// Start admits a request: it checks everything that can refuse it, reserves
// a stream and stores the learner's message. The error is a *Refusal.
func (s *Service) Start(owner string, v *session.Session, q *catalog.Question, qs *session.QState, in Input) (*Turn, error) {
	t, err := s.admit(owner, v, q, qs, in)
	// A refusal is counted here; a message that is admitted is counted
	// when its answer ends.
	var ref *Refusal
	if errors.As(err, &ref) {
		name := "none"
		if p, _ := s.cfg.Holder.Current(); p != nil {
			name = p.Name()
		}
		s.count(context.Background(), name, PhaseAdmission, ref.Code, v)
	}
	return t, err
}

func (s *Service) admit(owner string, v *session.Session, q *catalog.Question, qs *session.QState, in Input) (*Turn, error) {
	message := strings.TrimSpace(in.Message)
	if utf8.RuneCountInString(message) > MaxMessageChars {
		return nil, &Refusal{Status: 400, Code: CodeTooLong, Message: fmt.Sprintf("The message is too long (at most %d characters).", MaxMessageChars)}
	}
	if examRunning(v) {
		return nil, &Refusal{Status: 409, Code: CodeExamRunning, Message: "The assistant is available after the exam."}
	}
	provider, _ := s.cfg.Holder.Current()
	if provider == nil {
		return nil, &Refusal{Status: 409, Code: CodeNotConfigured, Message: "The assistant is not available. " + s.cfg.Holder.Problem()}
	}
	release, err := s.gate.Acquire(conversation(v, q))
	switch {
	case errors.Is(err, ratelimit.ErrBusy):
		return nil, &Refusal{Status: 409, Code: CodeBusy, Message: "The assistant is still answering the previous message of this question."}
	case err != nil:
		return nil, &Refusal{Status: 429, Code: CodeRateLimited, RetryAfter: 5 * time.Second, Message: "The assistant is busy with other answers. Try again in a few seconds."}
	}
	t := &Turn{s: s, v: v, q: q, qs: qs, provider: provider, release: release}
	commit, err := t.plan(message, in.RetryOf)
	if err == nil {
		// Only a request that will reach the model counts against the limit.
		if r := s.allow(owner); r != nil {
			err = r
		} else {
			err = commit()
		}
	}
	if err != nil {
		release()
		return nil, err
	}
	s.purgeOld()
	return t, nil
}

// plan settles which stored message is being answered and what came before
// it. It changes nothing: commit does, once the request is admitted.
func (t *Turn) plan(message string, retryOf int64) (commit func() error, err error) {
	st := t.s.cfg.Store
	rows, err := st.AssistantMessages(t.v.ID, t.q.ID)
	if err != nil {
		return nil, err
	}
	count := func() {
		t.turn = 1
		for _, m := range t.prior {
			if m.Role == "user" {
				t.turn++
			}
		}
	}
	if retryOf != 0 {
		last := len(rows) - 1
		found := -1
		for i, m := range rows {
			if m.ID == retryOf {
				found = i
			}
		}
		switch {
		case found < 0:
			return nil, &Refusal{Status: 404, Code: CodeNoMessage, Message: "There is no such message in this conversation."}
		case rows[found].Role != "assistant" || found != last || found == 0 || rows[found-1].Role != "user":
			return nil, &Refusal{Status: 400, Code: CodeBadRetry, Message: "Only the assistant's last answer can be asked for again."}
		}
		question := rows[last-1]
		t.prior, t.message, t.userID = rows[:last-1], question.Content, question.ID
		count()
		return func() error { return st.DeleteAssistantMessage(t.v.ID, t.q.ID, retryOf) }, nil
	}
	if message == "" {
		message = ai.DefaultQuestion
	}
	t.message = message
	// A message that got no answer and is sent again is the same message.
	if n := len(rows); n > 0 && rows[n-1].Role == "user" && rows[n-1].Content == message {
		t.prior, t.userID = rows[:n-1], rows[n-1].ID
		count()
		return func() error { return nil }, nil
	}
	t.prior = rows
	count()
	return func() error {
		id, err := st.AddAssistantMessage(store.AssistantMessage{SessionID: t.v.ID, QuestionID: t.q.ID, Role: "user", Content: message})
		t.userID = id
		return err
	}, nil
}

// buildMessages turns the stored conversation and the new message into what
// the model gets: earlier turns as plain text, strictly alternating and
// starting with the learner, at most maxTurns exchanges, and last the new
// message with the context block in front. final makes that last message
// from what the learner wrote.
func buildMessages(prior []store.AssistantMessage, message string, maxTurns int, final func(message string) string) []ai.Message {
	var msgs []ai.Message
	for _, m := range prior {
		text := strings.TrimSpace(m.Content)
		if text == "" || m.Role != "user" && m.Role != "assistant" {
			continue
		}
		if n := len(msgs); n > 0 && msgs[n-1].Role == m.Role {
			msgs[n-1].Content += "\n\n" + text
			continue
		}
		msgs = append(msgs, ai.Message{Role: m.Role, Content: text})
	}
	// Messages of the learner that never got an answer belong to the new one.
	if n := len(msgs); n > 0 && msgs[n-1].Role == "user" {
		message = msgs[n-1].Content + "\n\n" + message
		msgs = msgs[:n-1]
	}
	if over := len(msgs) - 2*maxTurns; over > 0 {
		msgs = msgs[over:]
	}
	for len(msgs) > 0 && msgs[0].Role != "user" {
		msgs = msgs[1:]
	}
	return append(msgs, ai.Message{Role: "user", Content: final(message)})
}

// attrs are the attributes of a span of this turn: what every span of
// session work says, the question, and more.
func (t *Turn) attrs(more ...span.Attr) []span.Attr {
	return append([]span.Attr{span.String("kubedojo.session.id", t.v.ID), span.String("kubedojo.track", t.v.Track),
		span.String("kubedojo.mode", t.v.Mode), span.String("kubedojo.question", t.q.ID)}, more...)
}

// Run answers the turn: it gathers the context, sends the conversation to
// the model, passes the answer on as events and stores it. It always ends
// with a Done or an Error event, unless the sink itself failed.
//
// The answer is the span assistant.chat, with the gathering of the context
// and the call to the model below it. It names the conversation and counts
// what went into it; what was said is not on it.
func (t *Turn) Run(ctx context.Context, sink Sink) {
	defer t.release()
	ctx, sp := span.Start(ctx, "assistant.chat", t.attrs(
		span.String("gen_ai.conversation.id", conversation(t.v, t.q)),
		span.Int("kubedojo.assistant.turn", t.turn))...)
	failed := t.answer(ctx, sink)
	result := ResultOK
	if failed != nil {
		result = string(failed.Kind)
	}
	sp.Set(span.String("kubedojo.assistant.result", result))
	// Counted inside the span, so the sample carries its trace.
	t.s.count(ctx, t.provider.Name(), PhaseAnswer, result, t.v)
	switch {
	case failed == nil, failed.Kind == ai.ErrRefused:
		// A model that declines has answered: no fault of the server.
		sp.End(nil)
	case failed.Kind == ai.ErrCanceled:
		// The learner left. As everywhere, that is a refusal, not an error.
		sp.Set(span.String("kubedojo.refused", "cancelled"))
		sp.End(nil)
	default:
		sp.Set(span.String("error.type", string(failed.Kind)))
		// The kind, the upstream status and the message the learner sees.
		// The cause stays in the log: it can name things of the upstream.
		sp.End(&ai.Error{Kind: failed.Kind, Status: failed.Status, Message: failed.Message})
	}
}

// answer does the work of Run. It returns the failure of the answer, or nil
// when the answer is complete.
func (t *Turn) answer(ctx context.Context, sink Sink) *ai.Error {
	s, start := t.s, time.Now()
	name, model := t.provider.Name(), t.provider.Model()
	gone := false
	send := func(e Event) {
		if !gone && sink(e) != nil {
			gone = true
		}
	}
	send(Meta{Conversation: conversation(t.v, t.q), Provider: name, Model: model, TraceID: span.TraceID(ctx), Turn: t.turn, UserMessageID: t.userID})

	send(Context{Stage: StageChecks})
	pc := t.gather(ctx, func(stage string) { send(Context{Stage: stage}) })

	req := ai.Request{System: ai.System, ConversationID: conversation(t.v, t.q),
		Messages: buildMessages(t.prior, t.message, MaxTurns, func(m string) string { return ai.UserTurn(pc, m) })}
	// How much of the conversation goes along with the new message.
	span.Current(ctx).Set(span.Int("kubedojo.assistant.history.messages", len(req.Messages)-1))
	var text strings.Builder
	var res ai.Result
	var err error
	if gone {
		err = &ai.Error{Kind: ai.ErrCanceled, Message: "The request was cancelled."}
	} else {
		res, err = t.provider.Chat(ctx, req, func(delta string) error {
			text.WriteString(delta)
			return sink(Delta{Text: delta})
		})
	}
	answer := text.String()
	if err == nil && strings.TrimSpace(answer) == "" {
		err = &ai.Error{Kind: ai.ErrUpstream, Message: "The model returned no text."}
	}
	add := func(partial bool) (int64, error) {
		return s.cfg.Store.AddAssistantMessage(store.AssistantMessage{SessionID: t.v.ID, QuestionID: t.q.ID, Role: "assistant", Content: answer, Partial: partial})
	}
	if err != nil {
		aerr := ai.AsError(err)
		if ctx.Err() == context.Canceled && aerr.Kind != ai.ErrCanceled {
			aerr = &ai.Error{Kind: ai.ErrCanceled, Message: "The request was cancelled.", Err: err}
		}
		out := Error{Code: aerr.Kind, Message: aerr.Message, Retryable: aerr.Kind.Retryable()}
		// What did arrive is kept, marked as cut short.
		if strings.TrimSpace(answer) != "" {
			if id, serr := add(true); serr == nil {
				out.MessageID = id
			} else {
				s.cfg.Log.ErrorContext(ctx, "assistant: the partial answer was not stored", "session", t.v.ID, "question", t.q.ID, "err", serr)
			}
		}
		level := slog.LevelWarn
		if aerr.Kind == ai.ErrCanceled {
			level = slog.LevelInfo
		}
		// The log line has the kind, the upstream status and the cause. It
		// never has the answer of the upstream server or a credential.
		s.cfg.Log.Log(ctx, level, "assistant: no answer", "session", t.v.ID, "question", t.q.ID, "provider", name, "model", model,
			"kind", string(aerr.Kind), "status", aerr.Status, "err", aerr.Err, "partial", out.MessageID != 0)
		send(out)
		return aerr
	}
	id, serr := add(false)
	if serr != nil {
		s.cfg.Log.ErrorContext(ctx, "assistant: the answer was not stored", "session", t.v.ID, "question", t.q.ID, "err", serr)
	}
	if res.Model == "" {
		res.Model = model
	}
	if res.FinishReason == "" {
		res.FinishReason = "stop"
	}
	send(Done{MessageID: id, FinishReason: res.FinishReason, Model: res.Model, Usage: res.Usage, Duration: time.Since(start)})
	return nil
}

// Chat is Start and Run in one call, for a caller that has nothing to do in
// between. The error is the *Refusal of a request that was not admitted;
// everything after that arrives as events.
func (s *Service) Chat(ctx context.Context, owner string, v *session.Session, q *catalog.Question, qs *session.QState, in Input, sink Sink) error {
	t, err := s.Start(owner, v, q, qs, in)
	if err != nil {
		return err
	}
	t.Run(ctx, sink)
	return nil
}

// History returns the stored conversation of a question, oldest first.
func (s *Service) History(v *session.Session, q *catalog.Question) ([]store.AssistantMessage, error) {
	if examRunning(v) {
		return nil, nil
	}
	return s.cfg.Store.AssistantMessages(v.ID, q.ID)
}

// Clear removes the conversation of a question.
func (s *Service) Clear(v *session.Session, q *catalog.Question) error {
	// Not while an answer of this conversation is arriving: it would be stored
	// after the clearing, without the question it answers.
	release, err := s.gate.Acquire(conversation(v, q))
	switch {
	case errors.Is(err, ratelimit.ErrBusy):
		return &Refusal{Status: 409, Code: CodeBusy, Message: "An answer is still arriving. Stop it, then start a new conversation."}
	case err == nil:
		defer release()
	}
	s.seen.drop(conversation(v, q) + "/")
	return s.cfg.Store.ClearAssistantMessages(v.ID, q.ID)
}
