// Package api exposes kubedojo over HTTP: REST for content and sessions, a
// WebSocket for the terminal, and the embedded single-page UI.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/checker"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
	"github.com/coder/websocket"
)

// Tutor explains failed checks (optional feature).
type Tutor interface {
	Explain(ctx context.Context, q *catalog.Question, task string, res *checker.Result, state string, ask string) (string, error)
}

type Server struct {
	Catalog  *catalog.Catalog
	Sessions *session.Manager
	Store    *store.Store
	UI       fs.FS
	Log      *slog.Logger
	Version  string
	Tutor    Tutor
	// UserHeader names a trusted reverse-proxy header carrying the user
	// identity (hosted mode behind an OIDC proxy). Empty = single local user.
	UserHeader string
	// AllowedOrigins are extra WebSocket origin patterns (dev server).
	AllowedOrigins []string
	// OnRating is called when a learner rates a question.
	OnRating func(track, question string, stars int)
	// OnTerminal is called when a terminal connects (delta +1) or leaves (-1).
	OnTerminal func(delta int)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/config", s.config)
	mux.HandleFunc("GET /api/tracks", s.tracks)
	mux.HandleFunc("GET /api/tracks/{track}", s.track)
	mux.HandleFunc("GET /api/tracks/{track}/lessons/{domain}", s.lesson)
	mux.HandleFunc("GET /api/tracks/{track}/progress", s.progress)
	mux.HandleFunc("POST /api/tracks/{track}/questions/{q}/rating", s.rate)
	mux.HandleFunc("GET /api/sessions", s.listSessions)
	mux.HandleFunc("POST /api/sessions", s.createSession)
	mux.HandleFunc("GET /api/sessions/{id}", s.getSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.deleteSession)
	mux.HandleFunc("POST /api/sessions/{id}/start", s.startSession)
	mux.HandleFunc("POST /api/sessions/{id}/finish", s.finishSession)
	mux.HandleFunc("GET /api/sessions/{id}/terminal", s.terminal)
	mux.HandleFunc("GET /api/sessions/{id}/questions/{q}", s.getQuestion)
	mux.HandleFunc("PATCH /api/sessions/{id}/questions/{q}", s.patchQuestion)
	mux.HandleFunc("POST /api/sessions/{id}/questions/{q}/prepare", s.prepare)
	mux.HandleFunc("POST /api/sessions/{id}/questions/{q}/check", s.check)
	mux.HandleFunc("GET /api/sessions/{id}/questions/{q}/hints/{n}", s.hint)
	mux.HandleFunc("GET /api/sessions/{id}/questions/{q}/solution", s.solution)
	mux.HandleFunc("POST /api/sessions/{id}/questions/{q}/tutor", s.tutor)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeErr(w, 404, "not found") })
	mux.Handle("/", s.spa())
	return mux
}

func (s *Server) spa() http.Handler {
	fileServer := http.FileServer(http.FS(s.UI))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.UI, p); err != nil {
			index, err := fs.ReadFile(s.UI, "index.html")
			if err != nil {
				http.Error(w, "UI not built. Run `make web` (or use a release binary).", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(index)
			return
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, session.ErrNotFound):
		writeErr(w, 404, "not found")
	case errors.Is(err, session.ErrConflict):
		writeErr(w, 409, err.Error())
	case errors.Is(err, session.ErrLimit):
		writeErr(w, 429, "Too many active sessions. End one before starting another.")
	default:
		s.Log.Error("request failed", "err", err)
		writeErr(w, 500, err.Error())
	}
}

func (s *Server) owner(r *http.Request) string {
	if s.UserHeader == "" {
		return ""
	}
	return r.Header.Get(s.UserHeader)
}

// session returns the session if it belongs to the caller.
func (s *Server) session(w http.ResponseWriter, r *http.Request) *session.Session {
	v, err := s.Sessions.View(r.PathValue("id"))
	if err == nil && v.Owner != s.owner(r) {
		err = session.ErrNotFound
	}
	if err != nil {
		s.fail(w, err)
		return nil
	}
	return v
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"version": s.Version, "tutor": s.Tutor != nil, "user": s.owner(r)})
}

type questionMeta struct {
	*catalog.Question
	Hints int `json:"hints"`
}

type trackView struct {
	*catalog.Track
	Questions []questionMeta `json:"questions"`
}

func viewTrack(t *catalog.Track) trackView {
	v := trackView{Track: t}
	for _, q := range t.Questions {
		v.Questions = append(v.Questions, questionMeta{Question: q, Hints: len(q.Hints)})
	}
	return v
}

func (s *Server) tracks(w http.ResponseWriter, r *http.Request) {
	var out []trackView
	for _, t := range s.Catalog.List() {
		out = append(out, viewTrack(t))
	}
	writeJSON(w, 200, out)
}

func (s *Server) track(w http.ResponseWriter, r *http.Request) {
	t := s.Catalog.Tracks[r.PathValue("track")]
	if t == nil {
		writeErr(w, 404, "unknown track")
		return
	}
	writeJSON(w, 200, viewTrack(t))
}

func (s *Server) lesson(w http.ResponseWriter, r *http.Request) {
	t := s.Catalog.Tracks[r.PathValue("track")]
	if t == nil {
		writeErr(w, 404, "unknown track")
		return
	}
	d := t.DomainByID(r.PathValue("domain"))
	if d == nil {
		writeErr(w, 404, "unknown domain")
		return
	}
	writeJSON(w, 200, map[string]any{"domain": d, "markdown": d.Lesson})
}

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
		ses, err = s.Sessions.NewExam(req.Track, session.ExamOptions{Minutes: req.Minutes, Questions: req.Questions, Owner: s.owner(r)})
	default:
		writeErr(w, 400, "mode must be practice or exam")
		return
	}
	if err != nil {
		s.fail(w, err)
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
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := s.Sessions.Destroy(ctx, v.ID); err != nil {
			s.Log.Error("destroy session", "session", v.ID, "err", err)
		}
	}()
	writeJSON(w, 202, map[string]string{"status": "destroying"})
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request) {
	v := s.session(w, r)
	if v == nil {
		return
	}
	if err := s.Sessions.Start(v.ID); err != nil {
		s.fail(w, err)
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
		s.fail(w, err)
		return
	}
	v, _ = s.Sessions.View(v.ID)
	writeJSON(w, 200, s.sessionView(v))
}

// render fills the per-session placeholders of content text.
func render(text, host string) string {
	if host == "" {
		host = "<host>"
	}
	return strings.NewReplacer("{{host}}", host, "{{worker}}", host+"-worker", "{{worker2}}", host+"-worker2").Replace(text)
}

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
	writeJSON(w, 200, map[string]any{
		"question": questionMeta{Question: q, Hints: len(q.Hints)},
		"task":     render(q.Task, host),
		"host":     host,
		"state":    qs,
	})
}

func (s *Server) patchQuestion(w http.ResponseWriter, r *http.Request) {
	v, q, _ := s.question(w, r)
	if v == nil {
		return
	}
	var req struct {
		Flagged *bool   `json:"flagged"`
		Note    *string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if err := s.Sessions.Patch(v.ID, q.ID, req.Flagged, req.Note); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) prepare(w http.ResponseWriter, r *http.Request) {
	v, q, _ := s.question(w, r)
	if v == nil {
		return
	}
	if err := s.Sessions.Prepare(v.ID, q.ID, r.URL.Query().Get("reset") == "1"); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 202, map[string]string{"status": "preparing"})
}

func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	v, q, _ := s.question(w, r)
	if v == nil {
		return
	}
	res, err := s.Sessions.Check(r.Context(), v.ID, q.ID)
	if err != nil {
		s.fail(w, err)
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
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"n": n, "markdown": render(q.Hints[n-1], v.Envs[q.Environment])})
}

func (s *Server) solution(w http.ResponseWriter, r *http.Request) {
	v, q, _ := s.question(w, r)
	if v == nil {
		return
	}
	if err := s.Sessions.Reveal(v.ID, q.ID, 0); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"markdown": render(q.SolutionMd, v.Envs[q.Environment])})
}

func (s *Server) tutor(w http.ResponseWriter, r *http.Request) {
	v, q, qs := s.question(w, r)
	if v == nil {
		return
	}
	if s.Tutor == nil {
		writeErr(w, 404, "The AI tutor is not configured. Set ANTHROPIC_API_KEY to enable it.")
		return
	}
	if v.Mode == session.ModeExam && v.Status != session.StatusFinished {
		writeErr(w, 409, "The tutor is available after the exam.")
		return
	}
	var req struct {
		Ask string `json:"ask"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	state := ""
	if qs.Status == "ready" {
		// A compact, read-only snapshot of what the learner built.
		res, err := s.Sessions.Exec(ctx, v.ID, q.ID, "control-plane", tutorSnapshot)
		if err == nil {
			state = res.Stdout
		}
	}
	text, err := s.Tutor.Explain(ctx, q, render(q.Task, v.Envs[q.Environment]), qs.Last, state, req.Ask)
	if err != nil {
		s.Log.Error("tutor", "err", err)
		writeErr(w, 502, "The tutor could not answer: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"markdown": text})
}

const tutorSnapshot = `
echo "## nodes"; kubectl get nodes -o wide 2>&1 | head -10
echo "## non-running pods"; kubectl get pods -A --field-selector=status.phase!=Running 2>&1 | head -20
echo "## recent warning events"; kubectl get events -A --field-selector=type=Warning --sort-by=.lastTimestamp 2>&1 | tail -15
`

func (s *Server) terminal(w http.ResponseWriter, r *http.Request) {
	v := s.session(w, r)
	if v == nil {
		return
	}
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.AllowedOrigins})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	tty, err := s.Sessions.Attach(ctx, v.ID, uint(cols), uint(rows))
	if err != nil {
		_ = c.Write(ctx, websocket.MessageBinary, []byte("\r\n\x1b[31mCould not open terminal: "+err.Error()+"\x1b[0m\r\n"))
		c.Close(websocket.StatusInternalError, "attach failed")
		return
	}
	defer tty.Close()
	if s.OnTerminal != nil {
		s.OnTerminal(1)
		defer s.OnTerminal(-1)
	}

	go func() { // sandbox -> browser
		defer cancel()
		buf := make([]byte, 32*1024)
		for {
			n, err := tty.Read(buf)
			if n > 0 {
				if c.Write(ctx, websocket.MessageBinary, buf[:n]) != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() { // keep-alive through proxies
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, pc := context.WithTimeout(ctx, 10*time.Second)
				err := c.Ping(pctx)
				pc()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	for { // browser -> sandbox
		typ, data, err := c.Read(ctx)
		if err != nil {
			break
		}
		if typ == websocket.MessageText {
			var msg struct {
				Type string `json:"type"`
				Cols uint   `json:"cols"`
				Rows uint   `json:"rows"`
			}
			if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" && msg.Cols > 0 && msg.Rows > 0 {
				_ = tty.Resize(ctx, msg.Cols, msg.Rows)
			}
			continue
		}
		if _, err := tty.Write(data); err != nil {
			break
		}
	}
	c.Close(websocket.StatusNormalClosure, "")
}

func (s *Server) rate(w http.ResponseWriter, r *http.Request) {
	t := s.Catalog.Tracks[r.PathValue("track")]
	if t == nil || t.Question(r.PathValue("q")) == nil {
		writeErr(w, 404, "unknown question")
		return
	}
	var req struct {
		Stars   int    `json:"stars"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil || req.Stars < 1 || req.Stars > 5 {
		writeErr(w, 400, "stars must be between 1 and 5")
		return
	}
	comment := strings.TrimSpace(req.Comment)
	if len(comment) > 2000 {
		comment = comment[:2000]
	}
	q := t.Question(r.PathValue("q"))
	if err := s.Store.AddRating(store.Rating{Track: t.ID, QuestionID: q.ID, Owner: s.owner(r), Stars: req.Stars, Comment: comment, At: time.Now()}); err != nil {
		s.fail(w, err)
		return
	}
	if s.OnRating != nil {
		s.OnRating(t.ID, q.ID, req.Stars)
	}
	writeJSON(w, 201, map[string]string{"status": "ok"})
}

// ---- progress -------------------------------------------------------------

type domainProgress struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Weight    int     `json:"weight"`
	Questions int     `json:"questions"`
	Attempted int     `json:"attempted"`
	Solved    int     `json:"solved"`
	Mastery   float64 `json:"mastery"` // 0..100, best score per question averaged over all questions
}

type questionProgress struct {
	Best     float64   `json:"best"` // 0..1
	Attempts int       `json:"attempts"`
	Last     time.Time `json:"last"`
}

type recommendation struct {
	Domain string `json:"domain"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
	Next   string `json:"next,omitempty"` // question id to practise next
}

func (s *Server) progress(w http.ResponseWriter, r *http.Request) {
	t := s.Catalog.Tracks[r.PathValue("track")]
	if t == nil {
		writeErr(w, 404, "unknown track")
		return
	}
	attempts, err := s.Store.Attempts(t.ID, s.owner(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	exams, err := s.Store.Exams(t.ID, s.owner(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	qp := map[string]*questionProgress{}
	for _, a := range attempts {
		if t.Question(a.QuestionID) == nil || a.Max == 0 {
			continue
		}
		p := qp[a.QuestionID]
		if p == nil {
			p = &questionProgress{}
			qp[a.QuestionID] = p
		}
		p.Attempts++
		p.Last = a.At
		if ratio := float64(a.Score) / float64(a.Max); ratio > p.Best {
			p.Best = ratio
		}
	}
	var domains []domainProgress
	var recs []recommendation
	type cand struct {
		prio float64
		rec  recommendation
	}
	var cands []cand
	overall := 0.0
	for _, d := range t.Domains {
		dp := domainProgress{ID: d.ID, Title: d.Title, Weight: d.Weight}
		sum := 0.0
		next := ""
		for _, q := range t.Questions {
			if q.Domain != d.ID {
				continue
			}
			dp.Questions++
			p := qp[q.ID]
			if p != nil {
				dp.Attempted++
				sum += p.Best
				if p.Best >= 1 {
					dp.Solved++
				}
			}
			if next == "" && (p == nil || p.Best < 1) {
				next = q.ID
			}
		}
		if dp.Questions > 0 {
			dp.Mastery = float64(int(sum/float64(dp.Questions)*1000+0.5)) / 10
		}
		overall += dp.Mastery * float64(d.Weight) / 100
		domains = append(domains, dp)
		if dp.Questions > 0 && dp.Mastery < 100 {
			reason := "Not practised yet"
			if dp.Attempted > 0 {
				reason = strconv.Itoa(dp.Solved) + " of " + strconv.Itoa(dp.Questions) + " questions solved"
			}
			reason += " · worth " + strconv.Itoa(d.Weight) + "% of the exam"
			cands = append(cands, cand{prio: float64(d.Weight) * (100 - dp.Mastery), rec: recommendation{Domain: d.ID, Title: d.Title, Reason: reason, Next: next}})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].prio > cands[j].prio })
	for i, c := range cands {
		if i == 3 {
			break
		}
		recs = append(recs, c.rec)
	}
	if exams == nil {
		exams = []store.Exam{}
	}
	writeJSON(w, 200, map[string]any{
		"track":           t.ID,
		"readiness":       float64(int(overall*10+0.5)) / 10,
		"passPercent":     t.Exam.PassPercent,
		"domains":         domains,
		"questions":       qp,
		"exams":           exams,
		"recommendations": recs,
		"attempts":        len(attempts),
	})
}
