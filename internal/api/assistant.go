package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/checker"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/session"
)

// Tutor explains failed checks (optional feature).
type Tutor interface {
	Explain(ctx context.Context, q *catalog.Question, task string, res *checker.Result, state string, ask string) (string, error)
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
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, 400, "invalid JSON")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	state := ""
	if qs.Status == "ready" && q.Type != catalog.TypeChoice {
		// A compact, read-only snapshot of what the learner built, taken on
		// the main machine of the question's environment.
		res, err := s.Sessions.Exec(ctx, v.ID, q.ID, "", s.tutorSnapshot(q))
		if err == nil {
			state = res.Stdout
		}
	}
	text, err := s.Tutor.Explain(ctx, q, render(q.Task, v.Envs[q.Environment]), qs.Last, state, req.Ask)
	if err != nil {
		s.Log.ErrorContext(ctx, "tutor", "err", err)
		writeErr(w, 502, "The tutor could not answer: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"markdown": text})
}

const clusterSnapshot = `
echo "## nodes"; kubectl get nodes -o wide 2>&1 | head -10
echo "## non-running pods"; kubectl get pods -A --field-selector=status.phase!=Running 2>&1 | head -20
echo "## recent warning events"; kubectl get events -A --field-selector=type=Warning --sort-by=.lastTimestamp 2>&1 | tail -15
`

// hostSnapshot describes a Linux host: there is no kubectl on it. It looks at
// what an exercise owns by convention, /opt/course/<question id>/.
const hostSnapshot = `
echo "## exercise directory /opt/course/$KD_QUESTION"; ls -la "/opt/course/$KD_QUESTION" 2>&1 | head -30
echo "## processes"; ps -eo user,pid,ppid,stat,args --sort=pid 2>&1 | head -30
echo "## listening sockets"; ss -ltnp 2>&1 | head -15
`

// tutorSnapshot picks the snapshot script that fits the question's environment.
func (s *Server) tutorSnapshot(q *catalog.Question) string {
	if t := s.Catalog.Tracks[q.Track]; t != nil {
		if e := t.Environments[q.Environment]; e != nil && e.Type == sandbox.EnvContainer {
			return hostSnapshot
		}
	}
	return clusterSnapshot
}
