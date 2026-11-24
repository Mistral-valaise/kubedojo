package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

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
	if err := readJSON(w, r, &req, 8<<10); err != nil || req.Stars < 1 || req.Stars > 5 {
		writeErr(w, 400, "stars must be between 1 and 5")
		return
	}
	comment := strings.TrimSpace(req.Comment)
	if len(comment) > 2000 {
		comment = comment[:2000]
	}
	q := t.Question(r.PathValue("q"))
	if err := s.Store.AddRating(store.Rating{Track: t.ID, QuestionID: q.ID, Owner: s.owner(r), Stars: req.Stars, Comment: comment, At: time.Now()}); err != nil {
		s.fail(w, r, err)
		return
	}
	if s.OnRating != nil {
		s.OnRating(r.Context(), t.ID, q.ID, req.Stars)
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
	Best float64 `json:"best"` // 0..1
	// Latest is the score of the most recent attempt. It is what counts for a
	// choice question: having picked the right option once, perhaps after
	// seeing the answer, says little about knowing it now.
	Latest   float64   `json:"latest"`
	Attempts int       `json:"attempts"`
	Last     time.Time `json:"last"`
}

type recommendation struct {
	Domain string `json:"domain"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
	Next   string `json:"next,omitempty"` // question id to practise next
}

// recommendationReason says why a domain is recommended, in the words of the
// track's kind: a skills track has exercises and a skills assessment, not
// questions and an exam.
func recommendationReason(kind string, dp domainProgress, weight int) string {
	items, whole := "questions", "exam"
	if kind == catalog.KindSkills {
		items, whole = "exercises", "skills assessment"
	}
	reason := "Not practised yet"
	if dp.Attempted > 0 {
		reason = strconv.Itoa(dp.Solved) + " of " + strconv.Itoa(dp.Questions) + " " + items + " solved"
	}
	return reason + " · worth " + strconv.Itoa(weight) + "% of the " + whole
}

func (s *Server) progress(w http.ResponseWriter, r *http.Request) {
	t := s.Catalog.Tracks[r.PathValue("track")]
	if t == nil {
		writeErr(w, 404, "unknown track")
		return
	}
	attempts, err := s.Store.Attempts(t.ID, s.owner(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	exams, err := s.Store.Exams(t.ID, s.owner(r))
	if err != nil {
		s.fail(w, r, err)
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
		p.Latest = float64(a.Score) / float64(a.Max)
		p.Best = max(p.Best, p.Latest)
	}
	// score is what a question contributes to mastery and readiness.
	score := func(q *catalog.Question) (float64, bool) {
		p := qp[q.ID]
		switch {
		case p == nil:
			return 0, false
		case q.Type == catalog.TypeChoice:
			return p.Latest, true
		}
		return p.Best, true
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
			got, attempted := score(q)
			if attempted {
				dp.Attempted++
				sum += got
				if got >= 1 {
					dp.Solved++
				}
			}
			if next == "" && got < 1 {
				next = q.ID
			}
		}
		if dp.Questions > 0 {
			dp.Mastery = float64(int(sum/float64(dp.Questions)*1000+0.5)) / 10
		}
		overall += dp.Mastery * float64(d.Weight) / 100
		domains = append(domains, dp)
		if dp.Questions > 0 && dp.Mastery < 100 {
			reason := recommendationReason(t.Kind, dp, d.Weight)
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
