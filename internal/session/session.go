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

func (s *Session) aliases() []string {
	out := make([]string, 0, len(s.Envs))
	for _, a := range s.Envs {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
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
type Hooks struct {
	Provisioned func(track, mode string, d time.Duration, err error)
	Checked     func(track, question, mode string, res checker.Result)
	ExamDone    func(track string, r Report)
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
	mu     sync.Mutex
	byID   map[string]*Session
	stopCh chan struct{}
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
	Hooks   Hooks
}

func NewManager(c Config) (*Manager, error) {
	m := &Manager{cat: c.Catalog, st: c.Store, drv: c.Driver, hosts: c.Hosts, log: c.Logger,
		max: c.MaxSessions, ttl: c.IdleTTL, hooks: c.Hooks, byID: map[string]*Session{}, stopCh: make(chan struct{})}
	if m.hosts == nil {
		m.hosts = LocalHosts()
	}
	if m.log == nil {
		m.log = slog.Default()
	}
	if m.max == 0 {
		m.max = 3
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
			s.Status, s.Error = StatusError, "interrupted by a server restart"
		}
		for _, q := range s.Questions {
			if q.Status == "preparing" {
				q.Status, q.Error = "error", "interrupted by a server restart"
			}
		}
		if s.TouchedAt.IsZero() {
			s.TouchedAt = time.Now().UTC()
		}
		m.byID[s.ID] = s
	}
	go m.clock()
	return m, nil
}

func (m *Manager) Close() { close(m.stopCh) }

// clock ends exams whose time is up.
func (m *Manager) clock() {
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
				m.log.Info("removing idle sandbox", "session", id)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				_ = m.Destroy(ctx, id)
				cancel()
			}
			for _, id := range due {
				m.log.Info("exam time is up", "session", id)
				_ = m.Finish(context.Background(), id)
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

func (m *Manager) logf(s *Session) func(string, ...any) {
	return func(format string, a ...any) {
		msg := fmt.Sprintf(format, a...)
		m.log.Info(msg, "session", s.ID)
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
		if s.Status != StatusDestroyed {
			n++
		}
	}
	return n
}

// Practice returns the active practice session of a track, creating it if needed.
func (m *Manager) Practice(track, owner string) (*Session, error) {
	if m.cat.Tracks[track] == nil {
		return nil, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.byID {
		if s.Mode == ModePractice && s.Track == track && s.Owner == owner && s.Status != StatusDestroyed {
			return s, nil
		}
	}
	if m.activeCount() >= m.max {
		return nil, ErrLimit
	}
	keys, err := sandbox.NewKeyPair()
	if err != nil {
		return nil, err
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
	keys, err := sandbox.NewKeyPair()
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.activeCount() >= m.max {
		m.mu.Unlock()
		return nil, ErrLimit
	}
	s := &Session{ID: newID(), Mode: ModeExam, Track: track, Status: StatusProvisioning, CreatedAt: time.Now().UTC(), TouchedAt: time.Now().UTC(),
		DurationMinutes: opt.Minutes, Envs: map[string]string{}, Keys: keys, Owner: opt.Owner}
	for _, q := range picked {
		s.Questions = append(s.Questions, &QState{ID: q.ID, Status: "pending"})
	}
	m.byID[s.ID] = s
	m.save(s)
	m.mu.Unlock()

	go m.provisionExam(s, t)
	return s, nil
}

// pickQuestions selects n questions following the domain weights of the track
// (largest remainder method), falling back to other domains when one runs dry.
func pickQuestions(t *catalog.Track, n int, driver string) []*catalog.Question {
	byDomain := map[string][]*catalog.Question{}
	total := 0
	for _, q := range t.Questions {
		if q.Driver == driver {
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

func (m *Manager) provisionExam(s *Session, t *catalog.Track) {
	s.op.Lock()
	defer s.op.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	logf := m.logf(s)
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
			logf("Preparing question %d of %d ...", i+1, len(order))
			if err := m.setup(ctx, s, t.Question(qs.ID), false); err != nil {
				return fmt.Errorf("question %s: %w", qs.ID, err)
			}
			m.update(s, func() { qs.Status = "ready" })
		}
		return nil
	}()
	if m.hooks.Provisioned != nil {
		m.hooks.Provisioned(s.Track, s.Mode, time.Since(start), err)
	}
	if err != nil {
		logf("Provisioning failed: %v", err)
		m.update(s, func() { s.Status, s.Error = StatusError, err.Error() })
		return
	}
	logf("Environment ready in %s. Start the exam when you are.", time.Since(start).Round(time.Second))
	m.update(s, func() {
		if s.Status == StatusProvisioning {
			s.Status = StatusReady
		}
	})
}

// ensureEnv creates the cluster for an environment if the session lacks it and
// rewires the terminal. Callers must hold s.op.
func (m *Manager) ensureEnv(ctx context.Context, s *Session, t *catalog.Track, env string, logf func(string, ...any)) error {
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
	host, err := m.hosts.Acquire(ctx, s.ID, logf)
	if err != nil {
		return err
	}
	m.mu.Lock()
	alias := m.newAlias(t.ID)
	m.mu.Unlock()
	start := time.Now()
	if err := m.drv.CreateEnv(ctx, host, s.ID, alias, *spec, s.Keys, logf); err != nil {
		_ = m.drv.Destroy(context.Background(), host, "none", []string{alias})
		return err
	}
	m.update(s, func() { s.Envs[env] = alias })
	logf("Connecting the terminal to %s ...", alias)
	if err := m.drv.EnsureTerminal(ctx, host, s.ID, s.aliases(), s.Keys); err != nil {
		return err
	}
	logf("Cluster %s is ready (%s).", alias, time.Since(start).Round(time.Second))
	return nil
}

func (m *Manager) runner(s *Session, q *catalog.Question) checker.Runner {
	return func(ctx context.Context, role, script string) (sandbox.ExecResult, error) {
		return m.exec(ctx, s, q, role, script)
	}
}

func (m *Manager) exec(ctx context.Context, s *Session, q *catalog.Question, role, script string) (sandbox.ExecResult, error) {
	host, err := m.hosts.Acquire(ctx, s.ID, func(string, ...any) {})
	if err != nil {
		return sandbox.ExecResult{}, err
	}
	m.mu.Lock()
	alias := s.Envs[q.Environment]
	m.mu.Unlock()
	env := map[string]string{"KD_HOST": alias, "KD_QUESTION": q.ID}
	user := "root"
	if role == "terminal" {
		user = "candidate"
	} else {
		env["KUBECONFIG"] = "/etc/kubernetes/admin.conf"
	}
	return m.drv.Exec(ctx, host, s.ID, sandbox.Target{Alias: alias, Role: role}, user, script, env)
}

// setup runs cleanup.sh (when resetting) and setup.sh of a question.
func (m *Manager) setup(ctx context.Context, s *Session, q *catalog.Question, reset bool) error {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	if reset && q.Cleanup != "" {
		if _, err := m.exec(ctx, s, q, "control-plane", q.Cleanup); err != nil {
			return err
		}
	}
	if strings.TrimSpace(q.Setup) == "" {
		return nil
	}
	res, err := m.exec(ctx, s, q, "control-plane", q.Setup)
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
	wasReady := qs.Status == "ready" || qs.Status == "error" || qs.Last != nil
	qs.Status, qs.Error = "preparing", ""
	if reset {
		qs.Last = nil
	}
	m.save(s)
	m.mu.Unlock()

	go func() {
		s.op.Lock()
		defer s.op.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		logf := m.logf(s)
		start := time.Now()
		recreated := false
		var err error
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
			m.hooks.Provisioned(s.Track, s.Mode, time.Since(start), err)
		}
		if err == nil {
			err = m.healOthers(ctx, s, t, q, logf)
		}
		if err == nil {
			logf("Setting up question %q ...", q.Title)
			err = m.setup(ctx, s, q, (reset || wasReady) && !recreated)
		}
		m.update(s, func() {
			if err != nil {
				qs.Status, qs.Error = "error", err.Error()
			} else {
				qs.Status = "ready"
			}
		})
		if err != nil {
			logf("Could not prepare question: %v", err)
		} else {
			logf("Question %q is ready.", q.Title)
		}
	}()
	return nil
}

// dropEnv deletes the cluster of an environment so it can be created afresh.
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
	logf("Recreating cluster %s for a clean start ...", alias)
	if err := m.drv.DeleteEnv(ctx, host, alias); err != nil {
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

// healOthers restores the cluster from disruptive questions that were left
// open, so that the question being prepared starts on a healthy cluster.
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
		logf("Restoring the cluster from %q ...", o.Title)
		cctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
		res, err := m.exec(cctx, s, o, "control-plane", o.Cleanup)
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
	s, err := m.get(id)
	if err != nil {
		return nil, err
	}
	t := m.cat.Tracks[s.Track]
	q := t.Question(qid)
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
	res := checker.Run(ctx, q, m.runner(s, q))
	m.update(s, func() { qs.Last = &res })
	if mode == ModePractice {
		_ = m.st.AddAttempt(store.Attempt{SessionID: s.ID, Mode: mode, Track: s.Track, Domain: q.Domain,
			QuestionID: q.ID, Score: res.Score, Max: res.Max, At: res.At, Owner: s.Owner})
	}
	if m.hooks.Checked != nil {
		m.hooks.Checked(s.Track, q.ID, mode, res)
	}
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
	m.mu.Lock()
	if s.Mode != ModeExam || s.Status != StatusRunning {
		m.mu.Unlock()
		return fmt.Errorf("%w: session cannot be finished in state %s", ErrConflict, s.Status)
	}
	s.Status = StatusGrading
	now := time.Now().UTC()
	s.FinishedAt = &now
	m.save(s)
	m.mu.Unlock()

	go func() {
		s.op.Lock()
		defer s.op.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		t := m.cat.Tracks[s.Track]
		logf := m.logf(s)
		rep := Report{PassPercent: t.Exam.PassPercent}
		dom := map[string]*DomainScore{}
		for _, d := range t.Domains {
			dom[d.ID] = &DomainScore{ID: d.ID, Title: d.Title, Weight: d.Weight}
		}
		for i, qs := range s.Questions {
			q := t.Question(qs.ID)
			logf("Grading question %d of %d ...", i+1, len(s.Questions))
			res := checker.Run(ctx, q, m.runner(s, q))
			m.update(s, func() { qs.Last = &res })
			rep.Score += res.Score
			rep.Max += res.Max
			dom[q.Domain].Score += res.Score
			dom[q.Domain].Max += res.Max
			_ = m.st.AddAttempt(store.Attempt{SessionID: s.ID, Mode: ModeExam, Track: s.Track, Domain: q.Domain,
				QuestionID: q.ID, Score: res.Score, Max: res.Max, At: res.At, Owner: s.Owner})
			if m.hooks.Checked != nil {
				m.hooks.Checked(s.Track, q.ID, ModeExam, res)
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
		m.update(s, func() { s.Report, s.Status = &rep, StatusFinished })
		db, _ := json.Marshal(rep.Domains)
		_ = m.st.AddExam(store.Exam{SessionID: s.ID, Track: s.Track, Score: rep.Score, Max: rep.Max,
			Percent: rep.Percent, Passed: rep.Passed, FinishedAt: *s.FinishedAt, Domains: db, Owner: s.Owner})
		if m.hooks.ExamDone != nil {
			m.hooks.ExamDone(s.Track, rep)
		}
		logf("Exam graded: %d/%d points (%.1f%%).", rep.Score, rep.Max, rep.Percent)
	}()
	_ = ctx
	return nil
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
	s.op.Lock()
	defer s.op.Unlock()
	host, err := m.hosts.Acquire(ctx, s.ID, func(string, ...any) {})
	if err == nil {
		if derr := m.drv.Destroy(ctx, host, s.ID, s.aliases()); derr != nil {
			m.log.Warn("destroy sandbox", "session", s.ID, "err", derr)
		}
	}
	_ = m.hosts.Release(ctx, s.ID)
	m.update(s, func() { s.Status = StatusDestroyed })
	return nil
}

// Attach opens an interactive shell on the session's terminal.
func (m *Manager) Attach(ctx context.Context, id string, cols, rows uint) (sandbox.TTY, error) {
	s, err := m.get(id)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	n := len(s.Envs)
	m.mu.Unlock()
	if n == 0 {
		return nil, fmt.Errorf("%w: sandbox is not ready yet", ErrConflict)
	}
	host, err := m.hosts.Acquire(ctx, s.ID, func(string, ...any) {})
	if err != nil {
		return nil, err
	}
	return m.drv.Attach(ctx, host, s.ID, cols, rows)
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

// Exec runs a script for a question (used by `kubedojo content test`).
func (m *Manager) Exec(ctx context.Context, id, qid, role, script string) (sandbox.ExecResult, error) {
	s, err := m.get(id)
	if err != nil {
		return sandbox.ExecResult{}, err
	}
	q := m.cat.Tracks[s.Track].Question(qid)
	if q == nil {
		return sandbox.ExecResult{}, ErrNotFound
	}
	return m.exec(ctx, s, q, role, script)
}
