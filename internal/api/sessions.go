package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/session"
)

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	out := s.Sessions.List(s.owner(r))
	if out == nil {
		out = []*session.Session{}
	}
	for _, v := range out {
		v.Log = nil
	}
	writeJSON(w, 200, out)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode      string `json:"mode"`
		Track     string `json:"track"`
		Minutes   int    `json:"minutes"`
		Questions int    `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	var (
		ses *session.Session
		err error
	)
	switch req.Mode {
	case session.ModePractice:
		ses, err = s.Sessions.Practice(req.Track, s.owner(r))
	case session.ModeExam:
		ses, err = s.Sessions.NewExamContext(r.Context(), req.Track, session.ExamOptions{Minutes: req.Minutes, Questions: req.Questions, Owner: s.owner(r)})
	default:
		writeErr(w, 400, "mode must be practice or exam")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v, _ := s.Sessions.View(ses.ID)
	writeJSON(w, 201, s.sessionView(v))
}

type sessionView struct {
	*session.Session
	RemainingSeconds *int              `json:"remainingSeconds,omitempty"`
	Now              time.Time         `json:"now"`
	Meta             map[string]qBrief `json:"meta"`
}

type qBrief struct {
	Title  string `json:"title"`
	Domain string `json:"domain"`
	Points int    `json:"points"`
	Host   string `json:"host"`
}

func (s *Server) sessionView(v *session.Session) sessionView {
	out := sessionView{Session: v, Now: time.Now().UTC(), Meta: map[string]qBrief{}}
	if v.Status == session.StatusRunning && v.Deadline != nil {
		rem := max(int(time.Until(*v.Deadline).Seconds()), 0)
		out.RemainingSeconds = &rem
	}
	t := s.Catalog.Tracks[v.Track]
	hide := v.Mode == session.ModeExam && v.Status != session.StatusFinished
	for _, qs := range v.Questions {
		if q := t.Question(qs.ID); q != nil {
			out.Meta[qs.ID] = qBrief{Title: q.Title, Domain: q.Domain, Points: q.Points, Host: v.Envs[q.Environment]}
		}
		if hide {
			qs.Last = nil // no grading feedback while the exam is running
		}
	}
	return out
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	if v := s.session(w, r); v != nil {
		writeJSON(w, 200, s.sessionView(v))
	}
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	v := s.session(w, r)
	if v == nil {
		return
	}
	if err := s.Sessions.DestroyLater(r.Context(), v.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 202, map[string]string{"status": "destroying"})
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request) {
	v := s.session(w, r)
	if v == nil {
		return
	}
	if err := s.Sessions.Start(v.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	v, _ = s.Sessions.View(v.ID)
	writeJSON(w, 200, s.sessionView(v))
}

func (s *Server) finishSession(w http.ResponseWriter, r *http.Request) {
	v := s.session(w, r)
	if v == nil {
		return
	}
	if err := s.Sessions.Finish(r.Context(), v.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	v, _ = s.Sessions.View(v.ID)
	writeJSON(w, 200, s.sessionView(v))
}
