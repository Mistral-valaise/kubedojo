package session

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/checker"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

// Multiple-choice tracks need no sandbox: their sessions never acquire a host
// and never call the driver. A session holds only what the learner did (the
// order the options were shown in and the options picked). Which options are
// correct stays in the catalog, because a Session is serialised as it is into
// SQLite and into API responses.

// ErrInvalid marks a request that can never succeed, whatever the state of
// the session: an answer naming an option the question does not have.
var ErrInvalid = errors.New("invalid")

// sandboxless reports whether the session belongs to a track without sandbox.
func (m *Manager) sandboxless(s *Session) bool {
	t := m.cat.Tracks[s.Track]
	return t != nil && t.Sandboxless()
}

// Revealed reports whether the learner may see the correct options and the
// explanations of a question: in practice once it was graded or its solution
// was opened, in an exam once the exam is finished.
func (s *Session) Revealed(qs *QState) bool {
	if s.Mode == ModeExam {
		return s.Status == StatusFinished
	}
	return qs != nil && (qs.Last != nil || qs.SolutionViewed)
}

// drawOrder returns the option ids in a fresh random order, so that neither
// the position in the file nor a remembered position gives the answer away.
func drawOrder(c *catalog.Choice) []string {
	ids := make([]string, len(c.Options))
	for i, o := range c.Options {
		ids[i] = o.ID
	}
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	return ids
}

// gradeChoice turns an answer into the same result a task produces: one point
// or none, and no checks.
func gradeChoice(q *catalog.Question, answer []string) checker.Result {
	res := checker.Result{QuestionID: q.ID, Max: q.Points, Checks: []checker.CheckResult{}, At: time.Now().UTC()}
	if q.Choice.Grade(answer) {
		res.Score, res.Passed = res.Max, true
	}
	return res
}

// choiceState returns the state of a choice question in a practice session,
// creating it and drawing the option order on first use. Callers hold m.mu.
func (s *Session) choiceState(q *catalog.Question) *QState {
	qs := s.q(q.ID)
	if qs == nil {
		qs = &QState{ID: q.ID}
		s.Questions = append(s.Questions, qs)
	}
	if len(qs.Order) == 0 {
		qs.Order = drawOrder(q.Choice)
	}
	qs.Status, qs.Error = "ready", ""
	return qs
}

// prepareChoice makes a choice question answerable. Nothing is provisioned,
// so it is done when it returns. A reset starts the question over: the answer,
// the result and the opened solution are cleared and the options reshuffled.
func (m *Manager) prepareChoice(s *Session, q *catalog.Question, reset bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if reset {
		if qs := s.q(q.ID); qs != nil {
			qs.Order, qs.Answer, qs.Last, qs.SolutionViewed = nil, nil, nil, false
		}
	}
	s.choiceState(q)
	m.save(s)
	return nil
}

// newQuizExam creates the exam of a sandboxless track. There is nothing to
// provision, so the session is ready at once.
func (m *Manager) newQuizExam(t *catalog.Track, opt ExamOptions, picked []*catalog.Question) *Session {
	now := time.Now().UTC()
	s := &Session{ID: newID(), Mode: ModeExam, Track: t.ID, Status: StatusReady, CreatedAt: now, TouchedAt: now,
		DurationMinutes: opt.Minutes, Envs: map[string]string{}, Owner: opt.Owner}
	for _, q := range picked {
		s.Questions = append(s.Questions, &QState{ID: q.ID, Status: "ready", Order: drawOrder(q.Choice)})
	}
	m.mu.Lock()
	m.byID[s.ID] = s
	m.save(s)
	m.mu.Unlock()
	return s
}

// Answer stores the options the learner picked for a choice question. It is
// allowed while the answer can still change the grade: in practice until the
// question is revealed, in an exam while the timer runs.
func (m *Manager) Answer(id, qid string, answer []string) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	q := m.cat.Tracks[s.Track].Question(qid)
	if q == nil {
		return ErrNotFound
	}
	if q.Type != catalog.TypeChoice {
		return fmt.Errorf("%w: question %s is answered in the terminal", ErrInvalid, qid)
	}
	seen := map[string]bool{}
	for _, a := range answer {
		if seen[a] {
			return fmt.Errorf("%w: option %q is given twice", ErrInvalid, a)
		}
		seen[a] = true
		if !slices.ContainsFunc(q.Choice.Options, func(o *catalog.Option) bool { return o.ID == a }) {
			return fmt.Errorf("%w: question %s has no option %q", ErrInvalid, qid, a)
		}
	}
	if !q.Choice.Multiple && len(answer) > 1 {
		return fmt.Errorf("%w: question %s takes one option, got %d", ErrInvalid, qid, len(answer))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var qs *QState
	if s.Mode == ModeExam {
		if qs = s.q(qid); qs == nil {
			return ErrNotFound
		}
		if s.Status != StatusRunning || (s.Deadline != nil && time.Now().After(*s.Deadline)) {
			return fmt.Errorf("%w: answers are taken while the exam is running", ErrConflict)
		}
	} else {
		if s.Revealed(s.q(qid)) {
			return fmt.Errorf("%w: the answer was already revealed; reset the question to try again", ErrConflict)
		}
		qs = s.choiceState(q)
	}
	qs.Answer = slices.Clone(answer)
	m.save(s)
	return nil
}

// checkChoice grades the stored answer of a choice question in practice. A
// graded question keeps its result until it is reset: grading it again would
// only add attempts for an answer that can no longer change.
func (m *Manager) checkChoice(s *Session, q *catalog.Question) (*checker.Result, error) {
	m.mu.Lock()
	if s.Mode == ModeExam {
		qs := s.q(q.ID)
		defer m.mu.Unlock()
		switch {
		case qs == nil:
			return nil, ErrNotFound
		case s.Status != StatusFinished || qs.Last == nil:
			return nil, fmt.Errorf("%w: during an exam, answers are graded when you finish", ErrConflict)
		}
		res := *qs.Last
		return &res, nil
	}
	qs := s.choiceState(q)
	if qs.Last != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: the question is already graded; reset it to try again", ErrConflict)
	}
	res := gradeChoice(q, qs.Answer)
	qs.Last = &res
	m.save(s)
	m.mu.Unlock()
	_ = m.st.AddAttempt(store.Attempt{SessionID: s.ID, Mode: s.Mode, Track: s.Track, Domain: q.Domain,
		QuestionID: q.ID, Score: res.Score, Max: res.Max, At: res.At, Owner: s.Owner})
	if m.hooks.Checked != nil {
		m.hooks.Checked(s.Track, q.ID, s.Mode, res)
	}
	return &res, nil
}

// finishQuiz grades the exam of a sandboxless track. Answers do not converge,
// so there is no settle time and the report exists when this returns. Results
// appear on the session together with the finished status, never before.
func (m *Manager) finishQuiz(s *Session) error {
	t := m.cat.Tracks[s.Track]
	m.mu.Lock()
	if s.Mode != ModeExam || s.Status != StatusRunning {
		m.mu.Unlock()
		return fmt.Errorf("%w: session cannot be finished in state %s", ErrConflict, s.Status)
	}
	s.Status = StatusGrading
	now := time.Now().UTC()
	s.FinishedAt = &now
	results := make([]checker.Result, len(s.Questions))
	for i, qs := range s.Questions {
		results[i] = gradeChoice(t.Question(qs.ID), qs.Answer)
	}
	m.save(s)
	m.mu.Unlock()
	m.conclude(s, t, results)
	return nil
}
