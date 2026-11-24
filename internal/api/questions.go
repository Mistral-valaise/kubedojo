package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/span"
)

// render fills the per-session placeholders of content text.
func render(text, host string) string {
	if host == "" {
		host = "<host>"
	}
	return strings.NewReplacer("{{host}}", host, "{{worker}}", host+"-worker", "{{worker2}}", host+"-worker2").Replace(text)
}

// question returns the session and the question of a request, and the state
// of the question in that session. It names the question on the span of the
// request, next to the session.
func (s *Server) question(w http.ResponseWriter, r *http.Request) (*session.Session, *catalog.Question, *session.QState) {
	v := s.session(w, r)
	if v == nil {
		return nil, nil, nil
	}
	q := s.Catalog.Tracks[v.Track].Question(r.PathValue("q"))
	if q == nil {
		writeErr(w, 404, "unknown question")
		return nil, nil, nil
	}
	span.Current(r.Context()).Set(span.String("kubedojo.question", q.ID))
	var qs *session.QState
	for _, x := range v.Questions {
		if x.ID == q.ID {
			qs = x
		}
	}
	if qs == nil {
		if v.Mode == session.ModeExam {
			writeErr(w, 404, "question is not part of this exam")
			return nil, nil, nil
		}
		qs = &session.QState{ID: q.ID, Status: "pending"}
	}
	return v, q, qs
}

// answered records on the span of the request that an answer to a choice
// question was stored. How many options were picked is recorded, never which:
// the answer is the learner's.
func answered(r *http.Request, answer []string) {
	span.Current(r.Context()).Event("question.answered", span.Int("options", len(answer)))
}

func (s *Server) getQuestion(w http.ResponseWriter, r *http.Request) {
	v, q, qs := s.question(w, r)
	if v == nil {
		return
	}
	host := v.Envs[q.Environment]
	if v.Mode == session.ModeExam && v.Status != session.StatusFinished {
		qs.Last = nil
	}
	if v.Mode == session.ModeExam && v.Status != session.StatusRunning && v.Status != session.StatusFinished {
		writeErr(w, 409, "the exam has not started yet")
		return
	}
	out := map[string]any{
		"question": questionMeta{Question: q, Hints: len(q.Hints)},
		"task":     render(q.Task, host),
		"host":     host,
		"state":    qs,
	}
	if q.Type == catalog.TypeChoice {
		if v.Mode == session.ModePractice && len(qs.Order) == 0 {
			// Preparing a choice question only draws the order of its options,
			// so a question opened for the first time is prepared on the spot.
			if v, qs = s.prepared(w, r, v, q); v == nil {
				return
			}
			out["state"] = qs
		}
		out["choice"] = viewChoice(v, q, qs)
	}
	writeJSON(w, 200, out)
}

// prepared prepares a choice question of a practice session and returns the
// session and the question's state as they are afterwards.
func (s *Server) prepared(w http.ResponseWriter, r *http.Request, v *session.Session, q *catalog.Question) (*session.Session, *session.QState) {
	err := s.Sessions.PrepareContext(r.Context(), v.ID, q.ID, false)
	if err == nil {
		v, err = s.Sessions.View(v.ID)
	}
	if err != nil {
		s.fail(w, r, err)
		return nil, nil
	}
	for _, x := range v.Questions {
		if x.ID == q.ID {
			return v, x
		}
	}
	s.fail(w, r, session.ErrNotFound)
	return nil, nil
}

func (s *Server) patchQuestion(w http.ResponseWriter, r *http.Request) {
	v, q, _ := s.question(w, r)
	if v == nil {
		return
	}
	var req struct {
		Flagged *bool     `json:"flagged"`
		Note    *string   `json:"note"`
		Answer  *[]string `json:"answer"`
	}
	if !s.decodeRequired(w, r, &req, 16<<10) {
		return
	}
	if req.Answer != nil {
		if err := s.Sessions.Answer(v.ID, q.ID, *req.Answer); err != nil {
			s.fail(w, r, err)
			return
		}
		answered(r, *req.Answer)
	}
	if req.Flagged != nil || req.Note != nil || req.Answer == nil {
		if err := s.Sessions.Patch(v.ID, q.ID, req.Flagged, req.Note); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) prepare(w http.ResponseWriter, r *http.Request) {
	v, q, _ := s.question(w, r)
	if v == nil {
		return
	}
	if err := s.Sessions.PrepareContext(r.Context(), v.ID, q.ID, r.URL.Query().Get("reset") == "1"); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 202, map[string]string{"status": "preparing"})
}

func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	v, q, _ := s.question(w, r)
	if v == nil {
		return
	}
	if q.Type == catalog.TypeChoice {
		// The body is optional: {"answer": [ids]} submits and grades in one call.
		var req struct {
			Answer *[]string `json:"answer"`
		}
		if !s.decode(w, r, &req, 8<<10) {
			return
		}
		if req.Answer != nil {
			if err := s.Sessions.Answer(v.ID, q.ID, *req.Answer); err != nil {
				s.fail(w, r, err)
				return
			}
			answered(r, *req.Answer)
		}
	}
	res, err := s.Sessions.Check(r.Context(), v.ID, q.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) hint(w http.ResponseWriter, r *http.Request) {
	v, q, _ := s.question(w, r)
	if v == nil {
		return
	}
	n, _ := strconv.Atoi(r.PathValue("n"))
	if n < 1 || n > len(q.Hints) {
		writeErr(w, 404, "no such hint")
		return
	}
	if err := s.Sessions.Reveal(v.ID, q.ID, n); err != nil {
		s.fail(w, r, err)
		return
	}
	span.Current(r.Context()).Event("hint.revealed", span.Int("n", n))
	writeJSON(w, 200, map[string]any{"n": n, "markdown": render(q.Hints[n-1], v.Envs[q.Environment])})
}

func (s *Server) solution(w http.ResponseWriter, r *http.Request) {
	v, q, _ := s.question(w, r)
	if v == nil {
		return
	}
	if q.Type == catalog.TypeChoice {
		// The solution of a choice question is its answer key. Opening it
		// reveals the question: it can be answered again only after a reset.
		if v.Mode == session.ModePractice {
			if v, _ = s.prepared(w, r, v, q); v == nil {
				return
			}
		}
		if err := s.Sessions.Reveal(v.ID, q.ID, 0); err != nil {
			s.fail(w, r, err)
			return
		}
		span.Current(r.Context()).Event("solution.revealed")
		writeJSON(w, 200, map[string]any{"markdown": solutionMarkdown(q.Choice), "review": review(q.Choice)})
		return
	}
	if err := s.Sessions.Reveal(v.ID, q.ID, 0); err != nil {
		s.fail(w, r, err)
		return
	}
	span.Current(r.Context()).Event("solution.revealed")
	writeJSON(w, 200, map[string]any{"markdown": render(q.SolutionMd, v.Envs[q.Environment])})
}
