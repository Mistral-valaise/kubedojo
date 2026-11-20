// Package session owns the lifecycle of practice and exam sessions: it
// provisions sandboxes, prepares questions, grades them and keeps score.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/checker"
	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

const (
	ModePractice = "practice"
	ModeExam     = "exam"

	StatusProvisioning = "provisioning"
	StatusReady        = "ready"   // sandbox is up; exam timer not started yet
	StatusRunning      = "running" // exam timer is running
	StatusGrading      = "grading"
	StatusFinished     = "finished"
	StatusError        = "error"
	StatusDestroyed    = "destroyed"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrLimit    = errors.New("session limit reached")
	// ErrShuttingDown is returned for work that would start while the server stops.
	ErrShuttingDown = fmt.Errorf("%w: the server is shutting down", ErrConflict)
)

type LogLine struct {
	At  time.Time `json:"at"`
	Msg string    `json:"msg"`
}

type QState struct {
	ID             string          `json:"id"`
	Status         string          `json:"status"` // pending | preparing | ready | error
	Error          string          `json:"error,omitempty"`
	Flagged        bool            `json:"flagged"`
	Note           string          `json:"note"`
	HintsUsed      int             `json:"hintsUsed"`
	SolutionViewed bool            `json:"solutionViewed"`
	Last           *checker.Result `json:"last,omitempty"`
	// Choice questions: the order the options are shown in and the options
	// the learner picked. Never the correct ones; see quiz.go.
	Order  []string `json:"order,omitempty"`
	Answer []string `json:"answer,omitempty"`
}

type DomainScore struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Weight int    `json:"weight"`
	Score  int    `json:"score"`
	Max    int    `json:"max"`
}

type Report struct {
	Score       int           `json:"score"`
	Max         int           `json:"max"`
	Percent     float64       `json:"percent"`
	PassPercent int           `json:"passPercent"`
	Passed      bool          `json:"passed"`
	Domains     []DomainScore `json:"domains"`
	UsedSeconds int           `json:"usedSeconds"`
}

type Session struct {
	ID              string            `json:"id"`
	Mode            string            `json:"mode"`
	Track           string            `json:"track"`
	Status          string            `json:"status"`
	Error           string            `json:"error,omitempty"`
	CreatedAt       time.Time         `json:"createdAt"`
	StartedAt       *time.Time        `json:"startedAt,omitempty"`
	Deadline        *time.Time        `json:"deadline,omitempty"`
	FinishedAt      *time.Time        `json:"finishedAt,omitempty"`
	DurationMinutes int               `json:"durationMinutes"`
	Envs            map[string]string `json:"envs"` // environment name -> host alias
	Questions       []*QState         `json:"questions"`
	Log             []LogLine         `json:"log"`
	Report          *Report           `json:"report,omitempty"`
	Keys            sandbox.KeyPair   `json:"keys"`
	Owner           string            `json:"owner,omitempty"`
	TouchedAt       time.Time         `json:"touchedAt"`

	op sync.Mutex // serialises long sandbox operations of this session
}

func (s *Session) q(id string) *QState {
	for _, q := range s.Questions {
		if q.ID == id {
			return q
		}
	}
	return nil
}

// envSpec returns the definition of an environment of a track. A session can
// outlive the content it was created from; what is unknown by now counts as a
// cluster, which is all there was before environments had a type.
func (m *Manager) envSpec(track, env string) sandbox.EnvSpec {
	if t := m.cat.Tracks[track]; t != nil {
		if e := t.Environments[env]; e != nil {
			return *e
		}
	}
	return sandbox.EnvSpec{Name: env, Type: sandbox.EnvCluster}
}

// envs lists the environments a session has created, ordered by alias.
func (m *Manager) envs(s *Session) []sandbox.Env {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]sandbox.Env, 0, len(s.Envs))
	for name, alias := range s.Envs {
		out = append(out, sandbox.Env{Alias: alias, Type: m.envSpec(s.Track, name).Type})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out
}

// noun names an environment in progress messages.
func noun(spec sandbox.EnvSpec) string {
	if spec.Type == sandbox.EnvContainer {
		return "host"
	}
	return "cluster"
}

// HostProvider decides which Docker endpoint a session's sandbox lives on.
type HostProvider interface {
	Acquire(ctx context.Context, sessionID string, logf func(string, ...any)) (dockerhost.Host, error)
	Release(ctx context.Context, sessionID string) error
}

type localHosts struct{}

func (localHosts) Acquire(context.Context, string, func(string, ...any)) (dockerhost.Host, error) {
	return dockerhost.Local(), nil
}
func (localHosts) Release(context.Context, string) error { return nil }

// LocalHosts runs every sandbox on the local Docker daemon.
func LocalHosts() HostProvider { return localHosts{} }

// Hooks let the telemetry layer observe session events without coupling.
// Every hook may be nil.
type Hooks struct {
	Provisioned func(ctx context.Context, track, mode string, d time.Duration, err error)
	Checked     func(ctx context.Context, track, question, mode string, res checker.Result)
	ExamDone    func(ctx context.Context, track string, r Report)
	// Span starts a span for a piece of work; attrs are pairs of key and
	// value. The returned function ends it.
	Span func(ctx context.Context, name string, attrs ...string) (context.Context, func(error))
	// Detach returns a context that lives as long as base and continues the
	// trace of request. Background work runs on it: it outlives the request
	// that started it and still shows up in that request's trace.
	Detach func(base, request context.Context) context.Context
}

type Manager struct {
	cat    *catalog.Catalog
	st     *store.Store
	drv    sandbox.Driver
	hosts  HostProvider
	log    *slog.Logger
	max    int
	ttl    time.Duration
	hooks  Hooks
	terms  terminals
	mu     sync.Mutex
	byID   map[string]*Session
	stopCh chan struct{}
	settle time.Duration
	// Background work (provisioning, preparing, grading, destroying, the clock)
	// runs on base and is counted in work, so that Shutdown can wait for it
	// and cancel what is left. closed is guarded by mu.
	base   context.Context
	cancel context.CancelFunc
	work   sync.WaitGroup
	closed bool
}

type Config struct {
	Catalog     *catalog.Catalog
	Store       *store.Store
	Driver      sandbox.Driver
	Hosts       HostProvider
	Logger      *slog.Logger
	MaxSessions int
	// IdleTTL removes sandboxes nobody looked at for this long (0 = never).
	IdleTTL time.Duration
	// GradeSettle is how long exam grading keeps re-running failed checks so
	// that state which is still converging (rollouts, endpoints) is graded
	// once it has settled. Zero means the default of 20s; negative disables it.
	GradeSettle time.Duration
	Hooks       Hooks
}

func NewManager(c Config) (*Manager, error) {
	m := &Manager{cat: c.Catalog, st: c.Store, drv: c.Driver, hosts: c.Hosts, log: c.Logger,
		max: c.MaxSessions, ttl: c.IdleTTL, hooks: c.Hooks, byID: map[string]*Session{}, stopCh: make(chan struct{})}
	m.base, m.cancel = context.WithCancel(context.Background())
	if m.hosts == nil {
		m.hosts = LocalHosts()
	}
	if m.log == nil {
		m.log = slog.Default()
	}
	if m.max == 0 {
		m.max = 3
	}
	m.settle = c.GradeSettle
	if m.settle == 0 {
		m.settle = 20 * time.Second
	}
	raws, err := c.Store.LoadSessions()
	if err != nil {
		return nil, err
	}
	for _, raw := range raws {
		s := &Session{}
		if json.Unmarshal(raw, s) != nil || s.ID == "" {
			continue
		}
		// Operations interrupted by a restart cannot be resumed.
		switch s.Status {
		case StatusProvisioning, StatusGrading:
			s.Status, s.Error = StatusError, interrupted
		}
		for _, q := range s.Questions {
			if q.Status == "preparing" {
				q.Status, q.Error = "error", interrupted
			}
		}
		if s.TouchedAt.IsZero() {
			s.TouchedAt = time.Now().UTC()
		}
		m.byID[s.ID] = s
	}
	m.work.Add(1)
	go m.clock()
	return m, nil
}

// interrupted is the error a session or question carries when the server
// stopped while it was being provisioned, prepared or graded.
const interrupted = "interrupted by a server restart"

// begin counts one piece of background work. It reports false once the server
// is shutting down; the caller must then not start it. The caller holds m.mu
// and calls m.work.Done when the work has ended.
func (m *Manager) begin() bool {
	if m.closed {
		return false
	}
	m.work.Add(1)
	return true
}

// span starts a span for work on a session. The session is an attribute of
// spans and log lines only; metrics never carry it.
func (m *Manager) span(ctx context.Context, name string, s *Session, attrs ...string) (context.Context, func(error)) {
	if m.hooks.Span == nil {
		return ctx, func(error) {}
	}
	return m.hooks.Span(ctx, name, append([]string{"kubedojo.session.id", s.ID, "kubedojo.track", s.Track, "kubedojo.mode", s.Mode}, attrs...)...)
}

// background returns the context of work that a request started and that
// goes on after the request was answered.
func (m *Manager) background(request context.Context) context.Context {
	if m.hooks.Detach == nil || request == nil {
		return m.base
	}
	return m.hooks.Detach(m.base, request)
}

// reason is what a failed operation tells the learner.
func (m *Manager) reason(err error) string {
	if m.base.Err() != nil {
		return interrupted
	}
	return err.Error()
}

// Shutdown stops the manager. It starts no new work, closes every terminal
// and waits for running operations — an exam being graded, a cluster being
// created — until ctx ends. What is still running then is cancelled and
// recorded as interrupted; Shutdown reports that with an error.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	ids := make([]string, 0, len(m.byID))
	for id := range m.byID {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	close(m.stopCh)
	for _, id := range ids {
		m.terms.closeAll(id)
	}
	done := make(chan struct{})
	go func() { m.work.Wait(); close(done) }()
	select {
	case <-done:
		m.cancel()
		return nil
	case <-ctx.Done():
	}
	m.cancel()
	// Cancelled operations notice within moments and persist their state.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	return fmt.Errorf("running operations were interrupted: %w", ctx.Err())
}

// Close stops the manager at once: running operations are cancelled.
func (m *Manager) Close() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = m.Shutdown(ctx)
}

// Running reports how many background operations of sessions are in progress.
func (m *Manager) Running() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, s := range m.byID {
		if s.Status == StatusProvisioning || s.Status == StatusGrading {
			n++
			continue
		}
		for _, q := range s.Questions {
			if q.Status == "preparing" {
				n++
				break
			}
		}
	}
	return n
}

// clock ends exams whose time is up.
func (m *Manager) clock() {
	defer m.work.Done()
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case now := <-t.C:
			m.mu.Lock()
			var due []string
			var idle []string
			for _, s := range m.byID {
				if s.Status == StatusRunning && s.Deadline != nil && now.After(*s.Deadline) {
					due = append(due, s.ID)
				}
				if m.ttl > 0 && s.Status != StatusDestroyed && s.Status != StatusRunning &&
					s.Status != StatusProvisioning && s.Status != StatusGrading && now.Sub(s.TouchedAt) > m.ttl {
					idle = append(idle, s.ID)
				}
			}
			m.mu.Unlock()
			for _, id := range idle {
				m.log.InfoContext(m.base, "removing idle sandbox", "session", id)
				ctx, cancel := context.WithTimeout(m.base, 5*time.Minute)
				_ = m.Destroy(ctx, id)
				cancel()
			}
			for _, id := range due {
				m.log.InfoContext(m.base, "exam time is up", "session", id)
				_ = m.Finish(m.base, id)
			}
		}
	}
}

func (m *Manager) save(s *Session) {
	if err := m.st.SaveSession(s.ID, s.Mode, s.Track, s.Status, s.CreatedAt, s); err != nil {
		m.log.Error("persist session", "session", s.ID, "err", err)
	}
}

// update mutates a session under the lock and persists it.
func (m *Manager) update(s *Session, fn func()) {
	m.mu.Lock()
	fn()
	m.save(s)
	m.mu.Unlock()
}

// logWords are the nouns of the progress log a learner reads while a session
// is set up or graded. They follow the kind of the track, like the UI does.
type logWords struct{ item, assessment string }

func wordsOf(t *catalog.Track) logWords {
	if t != nil && t.Kind == catalog.KindSkills {
		return logWords{item: "exercise", assessment: "assessment"}
	}
	return logWords{item: "question", assessment: "exam"}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// logf returns the progress log of a session: each line is shown to the
// learner and written to the server log, there with the trace of ctx.
func (m *Manager) logf(ctx context.Context, s *Session) func(string, ...any) {
	return func(format string, a ...any) {
		msg := fmt.Sprintf(format, a...)
		m.log.InfoContext(ctx, msg, "session", s.ID)
		m.update(s, func() {
			s.Log = append(s.Log, LogLine{At: time.Now().UTC(), Msg: msg})
			if len(s.Log) > 200 {
				s.Log = s.Log[len(s.Log)-200:]
			}
		})
	}
}

func (m *Manager) get(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byID[id]
	if !ok || s.Status == StatusDestroyed {
		return nil, ErrNotFound
	}
	s.TouchedAt = time.Now().UTC()
	return s, nil
}

func newID() string {
	const abc = "abcdefghjkmnpqrstuvwxyz23456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = abc[rand.IntN(len(abc))]
	}
	return string(b)
}

func (m *Manager) newAlias(track string) string {
	used := map[string]bool{}
	for _, s := range m.byID {
		for _, a := range s.Envs {
			used[a] = true
		}
	}
	for {
		a := fmt.Sprintf("%s%04d", track, 1000+rand.IntN(9000))
		if !used[a] {
			return a
		}
	}
}

func (m *Manager) activeCount() int {
	n := 0
	for _, s := range m.byID {
		// Sessions without a sandbox cost nothing and are not limited.
		if s.Status != StatusDestroyed && !m.sandboxless(s) {
			n++
		}
	}
	return n
}

// Practice returns the active practice session of a track, creating it if needed.
func (m *Manager) Practice(track, owner string) (*Session, error) {
	t := m.cat.Tracks[track]
	if t == nil {
		return nil, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.byID {
		if s.Mode == ModePractice && s.Track == track && s.Owner == owner && s.Status != StatusDestroyed {
			return s, nil
		}
	}
	var keys sandbox.KeyPair
	if !t.Sandboxless() {
		if m.activeCount() >= m.max {
			return nil, ErrLimit
		}
		var err error
		if keys, err = sandbox.NewKeyPair(); err != nil {
			return nil, err
		}
	}
	s := &Session{ID: newID(), Mode: ModePractice, Track: track, Status: StatusReady, CreatedAt: time.Now().UTC(),
		TouchedAt: time.Now().UTC(), Envs: map[string]string{}, Keys: keys, Owner: owner}
	m.byID[s.ID] = s
	m.save(s)
	return s, nil
}

type ExamOptions struct {
	Minutes   int
	Questions int
	Owner     string
}

// NewExam creates an exam session and provisions it in the background.
func (m *Manager) NewExam(track string, opt ExamOptions) (*Session, error) {
	return m.NewExamContext(context.Background(), track, opt)
}

// NewExamContext is NewExam for a request: the provisioning belongs to the
// trace of ctx. It is not cancelled with ctx.
func (m *Manager) NewExamContext(ctx context.Context, track string, opt ExamOptions) (*Session, error) {
	t := m.cat.Tracks[track]
	if t == nil {
		return nil, ErrNotFound
	}
	if opt.Minutes <= 0 {
		opt.Minutes = t.Exam.DurationMinutes
	}
	if opt.Questions <= 0 {
		opt.Questions = t.Exam.Questions
	}
	picked := pickQuestions(t, opt.Questions, m.drv.Name())
	if len(picked) == 0 {
		return nil, fmt.Errorf("track %s has no questions for driver %s", track, m.drv.Name())
	}
	if t.Sandboxless() {
		return m.newQuizExam(t, opt, picked), nil
	}
	keys, err := sandbox.NewKeyPair()
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.activeCount() >= m.max {
		m.mu.Unlock()
		return nil, ErrLimit
	}
	if !m.begin() {
		m.mu.Unlock()
		return nil, ErrShuttingDown
	}
	s := &Session{ID: newID(), Mode: ModeExam, Track: track, Status: StatusProvisioning, CreatedAt: time.Now().UTC(), TouchedAt: time.Now().UTC(),
		DurationMinutes: opt.Minutes, Envs: map[string]string{}, Keys: keys, Owner: opt.Owner}
	for _, q := range picked {
		s.Questions = append(s.Questions, &QState{ID: q.ID, Status: "pending"})
	}
	m.byID[s.ID] = s
	m.save(s)
	m.mu.Unlock()

	bg := m.background(ctx)
	go func() {
		defer m.work.Done()
		m.provisionExam(bg, s, t)
	}()
	return s, nil
}

// pickQuestions selects n questions following the domain weights of the track
// (largest remainder method), falling back to other domains when one runs dry.
func pickQuestions(t *catalog.Track, n int, driver string) []*catalog.Question {
	byDomain := map[string][]*catalog.Question{}
	total := 0
	for _, q := range t.Questions {
		if q.RunsOn(driver) {
			byDomain[q.Domain] = append(byDomain[q.Domain], q)
			total++
		}
	}
	if n > total {
		n = total
	}
	for _, qs := range byDomain {
		rand.Shuffle(len(qs), func(i, j int) { qs[i], qs[j] = qs[j], qs[i] })
	}
	type share struct {
		id   string
		n    int
		frac float64
	}
	var shares []share
	assigned := 0
	for _, d := range t.Domains {
		exact := float64(n) * float64(d.Weight) / 100
		sh := share{id: d.ID, n: int(exact), frac: exact - float64(int(exact))}
		assigned += sh.n
		shares = append(shares, sh)
	}
	sort.SliceStable(shares, func(i, j int) bool { return shares[i].frac > shares[j].frac })
	for i := 0; assigned < n; i = (i + 1) % len(shares) {
		shares[i].n++
		assigned++
	}
	var out, spare []*catalog.Question
	for _, sh := range shares {
		qs := byDomain[sh.id]
		k := min(sh.n, len(qs))
		out = append(out, qs[:k]...)
		spare = append(spare, qs[k:]...)
	}
	rand.Shuffle(len(spare), func(i, j int) { spare[i], spare[j] = spare[j], spare[i] })
	for len(out) < n && len(spare) > 0 {
		out, spare = append(out, spare[0]), spare[1:]
	}
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func (m *Manager) provisionExam(ctx context.Context, s *Session, t *catalog.Track) {
	s.op.Lock()
	defer s.op.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 25*time.Minute)
	defer cancel()
	ctx, end := m.span(ctx, "exam.provision", s)
	logf := m.logf(ctx, s)
	start := time.Now()
	err := func() error {
		envs := map[string]bool{}
		for _, qs := range s.Questions {
			envs[t.Question(qs.ID).Environment] = true
		}
		names := make([]string, 0, len(envs))
		for e := range envs {
			names = append(names, e)
		}
		sort.Strings(names)
		for _, e := range names {
			if err := m.ensureEnv(ctx, s, t, e, logf); err != nil {
				return err
			}
		}
		// Disruptive setups run last so they cannot disturb other setups.
		order := append([]*QState(nil), s.Questions...)
		sort.SliceStable(order, func(i, j int) bool {
			return !t.Question(order[i].ID).Disruptive && t.Question(order[j].ID).Disruptive
		})
		for i, qs := range order {
			logf("Preparing %s %d of %d ...", wordsOf(t).item, i+1, len(order))
			if err := m.setup(ctx, s, t.Question(qs.ID), false); err != nil {
				return fmt.Errorf("question %s: %w", qs.ID, err)
			}
			m.update(s, func() { qs.Status = "ready" })
		}
		return nil
	}()
	if m.hooks.Provisioned != nil {
		m.hooks.Provisioned(ctx, s.Track, s.Mode, time.Since(start), err)
	}
	end(err)
	if err != nil {
		logf("Provisioning failed: %v", err)
		m.update(s, func() { s.Status, s.Error = StatusError, m.reason(err) })
		return
	}
	logf("Environment ready in %s. Start the %s when you are.", time.Since(start).Round(time.Second), wordsOf(t).assessment)
	m.update(s, func() {
		if s.Status == StatusProvisioning {
			s.Status = StatusReady
		}
	})
}

// ensureEnv creates an environment (a cluster or a Linux host) if the session
// lacks it and rewires the terminal. Callers must hold s.op.
func (m *Manager) ensureEnv(ctx context.Context, s *Session, t *catalog.Track, env string, logf func(string, ...any)) (err error) {
	m.mu.Lock()
	_, ok := s.Envs[env]
	m.mu.Unlock()
	if ok {
		return nil
	}
	spec := t.Environments[env]
	if spec == nil {
		return fmt.Errorf("unknown environment %q", env)
	}
	ctx, end := m.span(ctx, "sandbox.provision", s, "kubedojo.environment", env)
	defer func() { end(err) }()
	// Where the time goes: a Pod in hosted mode, then the cluster or the
	// host, then the terminal.
	actx, aend := m.span(ctx, "sandbox.host.acquire", s)
	host, err := m.hosts.Acquire(actx, s.ID, logf)
	aend(err)
	if err != nil {
		return err
	}
	m.mu.Lock()
	alias := m.newAlias(t.ID)
	m.mu.Unlock()
	start := time.Now()
	cctx, cend := m.span(ctx, "sandbox.env.create", s, "kubedojo.environment", env)
	err = m.drv.CreateEnv(cctx, host, s.ID, alias, *spec, s.Keys, logf)
	cend(err)
	if err != nil {
		_ = m.drv.DeleteEnv(context.Background(), host, sandbox.Env{Alias: alias, Type: spec.Type})
		return err
	}
	m.update(s, func() { s.Envs[env] = alias })
	logf("Connecting the terminal to %s ...", alias)
	tctx, tend := m.span(ctx, "sandbox.terminal.connect", s)
	err = m.drv.EnsureTerminal(tctx, host, s.ID, m.envs(s), s.Keys)
	tend(err)
	if err != nil {
		return err
	}
	if spec.Type == sandbox.EnvContainer {
		logf("Host %s is ready (%s).", alias, time.Since(start).Round(100*time.Millisecond))
	} else {
		logf("Cluster %s is ready (%s).", alias, time.Since(start).Round(time.Second))
	}
	return nil
}

func (m *Manager) runner(s *Session, q *catalog.Question) checker.Runner {
	return func(ctx context.Context, role, script string) (sandbox.ExecResult, error) {
		return m.exec(ctx, s, q, role, "", script)
	}
}

// exec runs a script for a question. An empty role is the main machine of the
// question's environment; an empty user is the account scripts and checks use
// on that role.
func (m *Manager) exec(ctx context.Context, s *Session, q *catalog.Question, role, user, script string) (sandbox.ExecResult, error) {
	host, err := m.hosts.Acquire(ctx, s.ID, func(string, ...any) {})
	if err != nil {
		return sandbox.ExecResult{}, err
	}
	m.mu.Lock()
	alias := s.Envs[q.Environment]
	m.mu.Unlock()
	spec := m.envSpec(s.Track, q.Environment)
	if role == "" {
		role = spec.DefaultRole()
	}
	if user == "" {
		user = spec.ScriptUser(role)
	}
	env := map[string]string{"KD_HOST": alias, "KD_QUESTION": q.ID}
	if spec.Type != sandbox.EnvContainer && role != sandbox.RoleTerminal {
		env["KUBECONFIG"] = "/etc/kubernetes/admin.conf"
	}
	return m.drv.Exec(ctx, host, s.ID, sandbox.Target{Alias: alias, Role: role}, user, script, env)
}

// setup runs cleanup.sh (when resetting) and setup.sh of a question.
func (m *Manager) setup(ctx context.Context, s *Session, q *catalog.Question, reset bool) (err error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	ctx, end := m.span(ctx, "question.setup", s, "kubedojo.question", q.ID)
	defer func() { end(err) }()
	if reset && q.Cleanup != "" {
		if _, err := m.exec(ctx, s, q, "", "", q.Cleanup); err != nil {
			return err
		}
	}
	if strings.TrimSpace(q.Setup) == "" {
		return nil
	}
	res, err := m.exec(ctx, s, q, "", "", q.Setup)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		return fmt.Errorf("setup failed (exit %d): %s", res.ExitCode, msg)
	}
	return nil
}

// Prepare makes a question ready to be worked on in a practice session. It
// returns immediately; progress is visible on the session.
func (m *Manager) Prepare(id, qid string, reset bool) error {
	return m.PrepareContext(context.Background(), id, qid, reset)
}

// PrepareContext is Prepare for a request: the work belongs to the trace of
// ctx. It is not cancelled with ctx.
func (m *Manager) PrepareContext(ctx context.Context, id, qid string, reset bool) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	if s.Mode != ModePractice {
		return fmt.Errorf("%w: questions of an exam are prepared up front", ErrConflict)
	}
	t := m.cat.Tracks[s.Track]
	q := t.Question(qid)
	if q == nil {
		return ErrNotFound
	}
	if q.Type == catalog.TypeChoice {
		return m.prepareChoice(s, q, reset)
	}
	m.mu.Lock()
	qs := s.q(qid)
	if qs == nil {
		qs = &QState{ID: qid}
		s.Questions = append(s.Questions, qs)
	}
	if qs.Status == "preparing" || (qs.Status == "ready" && !reset) {
		m.mu.Unlock()
		return nil
	}
	if !m.begin() {
		m.mu.Unlock()
		return ErrShuttingDown
	}
	wasReady := qs.Status == "ready" || qs.Status == "error" || qs.Last != nil
	qs.Status, qs.Error = "preparing", ""
	if reset {
		qs.Last = nil
	}
	m.save(s)
	m.mu.Unlock()

	bg := m.background(ctx)
	go func() {
		defer m.work.Done()
		s.op.Lock()
		defer s.op.Unlock()
		ctx, cancel := context.WithTimeout(bg, 15*time.Minute)
		defer cancel()
		var err error
		ctx, end := m.span(ctx, "question.prepare", s, "kubedojo.question", q.ID)
		defer func() { end(err) }()
		logf := m.logf(ctx, s)
		start := time.Now()
		recreated := false
		if q.FreshEnvironment && wasReady {
			recreated = true
			err = m.dropEnv(ctx, s, t, q.Environment, logf)
		}
		m.mu.Lock()
		_, had := s.Envs[q.Environment]
		m.mu.Unlock()
		if err == nil {
			err = m.ensureEnv(ctx, s, t, q.Environment, logf)
		}
		if !had && m.hooks.Provisioned != nil {
			m.hooks.Provisioned(ctx, s.Track, s.Mode, time.Since(start), err)
		}
		if err == nil {
			err = m.healOthers(ctx, s, t, q, logf)
		}
		if err == nil {
			logf("Setting up %s %q ...", wordsOf(t).item, q.Title)
			err = m.setup(ctx, s, q, (reset || wasReady) && !recreated)
		}
		m.update(s, func() {
			if err != nil {
				qs.Status, qs.Error = "error", m.reason(err)
			} else {
				qs.Status = "ready"
			}
		})
		if err != nil {
			logf("Could not prepare %s: %v", wordsOf(t).item, err)
		} else {
			logf("%s %q is ready.", capitalize(wordsOf(t).item), q.Title)
		}
	}()
	return nil
}

// dropEnv deletes an environment so it can be created afresh.
// Every question that lived on it goes back to pending.
func (m *Manager) dropEnv(ctx context.Context, s *Session, t *catalog.Track, env string, logf func(string, ...any)) error {
	m.mu.Lock()
	alias := s.Envs[env]
	m.mu.Unlock()
	if alias == "" {
		return nil
	}
	host, err := m.hosts.Acquire(ctx, s.ID, logf)
	if err != nil {
		return err
	}
	spec := m.envSpec(s.Track, env)
	logf("Recreating %s %s for a clean start ...", noun(spec), alias)
	if err := m.drv.DeleteEnv(ctx, host, sandbox.Env{Alias: alias, Type: spec.Type}); err != nil {
		return err
	}
	m.update(s, func() {
		delete(s.Envs, env)
		for _, x := range s.Questions {
			if o := t.Question(x.ID); o != nil && o.Environment == env && x.Status == "ready" {
				x.Status = "pending"
			}
		}
	})
	return nil
}

// healOthers restores the environment from disruptive questions that were
// left open, so that the question being prepared starts on a healthy one.
func (m *Manager) healOthers(ctx context.Context, s *Session, t *catalog.Track, q *catalog.Question, logf func(string, ...any)) error {
	m.mu.Lock()
	var open []*QState
	for _, x := range s.Questions {
		o := t.Question(x.ID)
		if x.ID != q.ID && x.Status == "ready" && o != nil && o.Disruptive && o.Environment == q.Environment {
			open = append(open, x)
		}
	}
	m.mu.Unlock()
	for _, x := range open {
		o := t.Question(x.ID)
		logf("Restoring the %s from %q ...", noun(m.envSpec(s.Track, o.Environment)), o.Title)
		cctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
		res, err := m.exec(cctx, s, o, "", "", o.Cleanup)
		cancel()
		if err != nil {
			return err
		}
		if res.ExitCode != 0 {
			return fmt.Errorf("cleanup of %s failed: %s", o.ID, strings.TrimSpace(res.Stderr))
		}
		m.update(s, func() { x.Status = "pending" })
	}
	return nil
}

// Check grades one question of a practice session (or of a finished exam).
func (m *Manager) Check(ctx context.Context, id, qid string) (*checker.Result, error) {
	return m.CheckOnce(ctx, id, qid, nil)
}

// CheckOnce grades like Check, without retries for the checks once selects.
// See checker.RunOnce.
func (m *Manager) CheckOnce(ctx context.Context, id, qid string, once func(*catalog.Check) bool) (*checker.Result, error) {
	s, err := m.get(id)
	if err != nil {
		return nil, err
	}
	t := m.cat.Tracks[s.Track]
	q := t.Question(qid)
	if q != nil && q.Type == catalog.TypeChoice {
		return m.checkChoice(ctx, s, q)
	}
	m.mu.Lock()
	qs := s.q(qid)
	mode, status := s.Mode, s.Status
	m.mu.Unlock()
	if q == nil || qs == nil {
		return nil, ErrNotFound
	}
	if mode == ModeExam && status != StatusFinished {
		return nil, fmt.Errorf("%w: during an exam, answers are graded when you finish", ErrConflict)
	}
	if qs.Status != "ready" {
		return nil, fmt.Errorf("%w: question is not ready", ErrConflict)
	}
	ctx, end := m.span(ctx, "question.check", s, "kubedojo.question", q.ID)
	res := checker.RunOnce(ctx, q, m.runner(s, q), once)
	m.update(s, func() { qs.Last = &res })
	if mode == ModePractice {
		m.record(ctx, s, q, mode, res)
	}
	if m.hooks.Checked != nil {
		m.hooks.Checked(ctx, s.Track, q.ID, mode, res)
	}
	end(nil)
	return &res, nil
}

// Start begins the exam timer.
func (m *Manager) Start(id string) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.Mode != ModeExam || s.Status != StatusReady {
		return fmt.Errorf("%w: session cannot be started in state %s", ErrConflict, s.Status)
	}
	now := time.Now().UTC()
	dl := now.Add(time.Duration(s.DurationMinutes) * time.Minute)
	s.StartedAt, s.Deadline, s.Status = &now, &dl, StatusRunning
	m.save(s)
	return nil
}

// Finish grades every question of an exam and produces the report.
func (m *Manager) Finish(ctx context.Context, id string) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	if m.sandboxless(s) {
		return m.finishQuiz(ctx, s)
	}
	m.mu.Lock()
	if s.Mode != ModeExam || s.Status != StatusRunning {
		m.mu.Unlock()
		return fmt.Errorf("%w: session cannot be finished in state %s", ErrConflict, s.Status)
	}
	if !m.begin() {
		m.mu.Unlock()
		return ErrShuttingDown
	}
	s.Status = StatusGrading
	now := time.Now().UTC()
	s.FinishedAt = &now
	m.save(s)
	m.mu.Unlock()

	bg := m.background(ctx)
	go func() {
		defer m.work.Done()
		s.op.Lock()
		defer s.op.Unlock()
		ctx, cancel := context.WithTimeout(bg, 20*time.Minute)
		defer cancel()
		ctx, graded := m.span(ctx, "exam.grade", s)
		defer graded(nil)
		t := m.cat.Tracks[s.Track]
		logf := m.logf(ctx, s)
		results := make([]checker.Result, len(s.Questions))
		for i, qs := range s.Questions {
			q := t.Question(qs.ID)
			logf("Grading %s %d of %d ...", wordsOf(t).item, i+1, len(s.Questions))
			qctx, qend := m.span(ctx, "question.check", s, "kubedojo.question", q.ID)
			results[i] = checker.Run(qctx, q, m.runner(s, q))
			qend(nil)
			res := results[i]
			m.update(s, func() { qs.Last = &res })
		}
		// One shared window for everything that failed: answers given just
		// before the deadline may still be converging. The window is shared so
		// an unanswered exam is not graded for minutes.
		end := time.Now().Add(m.settle)
		for round := 1; m.settle > 0 && time.Now().Before(end); round++ {
			open := 0
			for i, qs := range s.Questions {
				if results[i].Passed || !time.Now().Before(end) {
					continue
				}
				if open == 0 {
					logf("Re-checking answers that were still settling (round %d) ...", round)
				}
				open++
				q := t.Question(qs.ID)
				results[i] = checker.Recheck(ctx, q, m.runner(s, q), results[i])
				res := results[i]
				m.update(s, func() { qs.Last = &res })
			}
			if open == 0 {
				break
			}
			select {
			case <-time.After(3 * time.Second):
			case <-ctx.Done():
			}
		}
		// Checks that were cut off by a shutdown failed for no fault of the
		// learner. Such an exam is not graded; it is marked as interrupted.
		if m.base.Err() != nil {
			m.update(s, func() { s.Status, s.Error = StatusError, interrupted })
			return
		}
		m.conclude(ctx, s, t, results)
	}()
	return nil
}

// record stores one graded question in the learner's history.
func (m *Manager) record(ctx context.Context, s *Session, q *catalog.Question, mode string, res checker.Result) {
	if err := m.st.AddAttempt(store.Attempt{SessionID: s.ID, Mode: mode, Track: s.Track, Domain: q.Domain,
		QuestionID: q.ID, Score: res.Score, Max: res.Max, At: res.At, Owner: s.Owner}); err != nil {
		m.log.ErrorContext(ctx, "the attempt was not recorded", "session", s.ID, "question", q.ID, "err", err)
	}
}

// DestroyLater tears the sandbox down in the background. The work belongs to
// the trace of ctx and is not cancelled with it.
func (m *Manager) DestroyLater(ctx context.Context, id string) error {
	if _, err := m.get(id); err != nil {
		return err
	}
	m.mu.Lock()
	ok := m.begin()
	m.mu.Unlock()
	if !ok {
		return ErrShuttingDown
	}
	bg := m.background(ctx)
	go func() {
		defer m.work.Done()
		ctx, cancel := context.WithTimeout(bg, 5*time.Minute)
		defer cancel()
		if err := m.Destroy(ctx, id); err != nil {
			m.log.ErrorContext(ctx, "destroy session", "session", id, "err", err)
		}
	}()
	return nil
}

// conclude turns the results of a graded exam into its report, records the
// attempts and marks the session finished.
func (m *Manager) conclude(ctx context.Context, s *Session, t *catalog.Track, results []checker.Result) {
	rep := Report{PassPercent: t.Exam.PassPercent}
	dom := map[string]*DomainScore{}
	for _, d := range t.Domains {
		dom[d.ID] = &DomainScore{ID: d.ID, Title: d.Title, Weight: d.Weight}
	}
	for i, qs := range s.Questions {
		q := t.Question(qs.ID)
		res := results[i]
		rep.Score += res.Score
		rep.Max += res.Max
		dom[q.Domain].Score += res.Score
		dom[q.Domain].Max += res.Max
		m.record(ctx, s, q, ModeExam, res)
		if m.hooks.Checked != nil {
			m.hooks.Checked(ctx, s.Track, q.ID, ModeExam, res)
		}
	}
	for _, d := range t.Domains {
		if dom[d.ID].Max > 0 {
			rep.Domains = append(rep.Domains, *dom[d.ID])
		}
	}
	if rep.Max > 0 {
		rep.Percent = float64(int(float64(rep.Score)/float64(rep.Max)*1000+0.5)) / 10
	}
	rep.Passed = rep.Percent >= float64(rep.PassPercent)
	if s.StartedAt != nil {
		rep.UsedSeconds = int(s.FinishedAt.Sub(*s.StartedAt).Seconds())
	}
	m.update(s, func() {
		for i, qs := range s.Questions {
			qs.Last = &results[i]
		}
		s.Report, s.Status = &rep, StatusFinished
	})
	db, _ := json.Marshal(rep.Domains)
	if err := m.st.AddExam(store.Exam{SessionID: s.ID, Track: s.Track, Score: rep.Score, Max: rep.Max,
		Percent: rep.Percent, Passed: rep.Passed, FinishedAt: *s.FinishedAt, Domains: db, Owner: s.Owner,
		Questions: len(s.Questions), Minutes: s.DurationMinutes}); err != nil {
		m.log.ErrorContext(ctx, "the exam result was not recorded", "session", s.ID, "err", err)
	}
	if m.hooks.ExamDone != nil {
		m.hooks.ExamDone(ctx, s.Track, rep)
	}
	m.logf(ctx, s)("%s graded: %d/%d points (%.1f%%).", capitalize(wordsOf(t).assessment), rep.Score, rep.Max, rep.Percent)
}

// Patch updates candidate-owned fields of a question.
func (m *Manager) Patch(id, qid string, flagged *bool, note *string) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	qs := s.q(qid)
	if qs == nil {
		return ErrNotFound
	}
	if flagged != nil {
		qs.Flagged = *flagged
	}
	if note != nil {
		qs.Note = *note
	}
	m.save(s)
	return nil
}

// Reveal records that a hint (n>0) or the solution (n==0) was opened and
// reports whether that is allowed right now.
func (m *Manager) Reveal(id, qid string, hint int) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	qs := s.q(qid)
	if qs == nil {
		return ErrNotFound
	}
	if s.Mode == ModeExam && s.Status != StatusFinished {
		return fmt.Errorf("%w: hints and solutions unlock after the exam", ErrConflict)
	}
	if hint == 0 {
		qs.SolutionViewed = true
	} else if hint > qs.HintsUsed {
		qs.HintsUsed = hint
	}
	m.save(s)
	return nil
}

// Destroy tears the sandbox down.
func (m *Manager) Destroy(ctx context.Context, id string) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	if m.sandboxless(s) {
		// Nothing was created. Acquiring a host here would create one in hosted mode.
		m.update(s, func() { s.Status = StatusDestroyed })
		return nil
	}
	s.op.Lock()
	defer s.op.Unlock()
	ctx, end := m.span(ctx, "sandbox.destroy", s)
	defer end(nil)
	// End the shells first: their slots are freed and the WebSockets close
	// instead of waiting for a read error from a container that is gone.
	m.terms.closeAll(s.ID)
	host, err := m.hosts.Acquire(ctx, s.ID, func(string, ...any) {})
	if err == nil {
		if derr := m.drv.Destroy(ctx, host, s.ID, m.envs(s)); derr != nil {
			m.log.WarnContext(ctx, "destroy sandbox", "session", s.ID, "err", derr)
		}
	}
	_ = m.hosts.Release(ctx, s.ID)
	m.update(s, func() { s.Status = StatusDestroyed })
	return nil
}

// View returns a deep copy that is safe to serialise (without secrets).
func (m *Manager) View(id string) (*Session, error) {
	s, err := m.get(id)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return clone(s), nil
}

func clone(s *Session) *Session {
	b, _ := json.Marshal(s)
	c := &Session{}
	_ = json.Unmarshal(b, c)
	c.Keys = sandbox.KeyPair{}
	return c
}

// List returns all live sessions, newest first.
func (m *Manager) List(owner string) []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Session
	for _, s := range m.byID {
		if s.Status != StatusDestroyed && s.Owner == owner {
			out = append(out, clone(s))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// ActiveCount is exported for metrics.
func (m *Manager) ActiveCount() (practice, exam int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.byID {
		if s.Status == StatusDestroyed {
			continue
		}
		if s.Mode == ModeExam {
			exam++
		} else {
			practice++
		}
	}
	return
}

// Exec runs a script for a question the way its setup and checks run: as the
// script account of the role. An empty role is the main machine of the
// question's environment.
func (m *Manager) Exec(ctx context.Context, id, qid, role, script string) (sandbox.ExecResult, error) {
	s, err := m.get(id)
	if err != nil {
		return sandbox.ExecResult{}, err
	}
	q := m.cat.Tracks[s.Track].Question(qid)
	if q == nil {
		return sandbox.ExecResult{}, ErrNotFound
	}
	if q.Type == catalog.TypeChoice {
		return sandbox.ExecResult{}, fmt.Errorf("%w: a choice question has no sandbox", ErrConflict)
	}
	return m.exec(ctx, s, q, role, "", script)
}

// Solve runs a solution of a question (solution.sh or a variant) on the main
// machine of its environment, as the account the environment says solutions
// use. `kubedojo content test` proves questions with it.
func (m *Manager) Solve(ctx context.Context, id, qid, script string) (sandbox.ExecResult, error) {
	s, err := m.get(id)
	if err != nil {
		return sandbox.ExecResult{}, err
	}
	q := m.cat.Tracks[s.Track].Question(qid)
	if q == nil {
		return sandbox.ExecResult{}, ErrNotFound
	}
	spec := m.envSpec(s.Track, q.Environment)
	return m.exec(ctx, s, q, spec.DefaultRole(), spec.SolveUser(), script)
}
