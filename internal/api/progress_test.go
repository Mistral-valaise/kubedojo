package api_test

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
)

// skillsTrack is a track of kind skills next to the quiz track: one exercise
// in a Linux container.
func skillsTrack() fstest.MapFS {
	f := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	dir := "skills/questions/q/"
	return fstest.MapFS{
		"skills/track.yaml": f("title: Skills\nkind: skills\nexam: {durationMinutes: 10, passPercent: 50, questions: 1}\n" +
			"domains: [{id: d, title: D, weight: 100}]\nenvironments: {base: {type: container}}\n"),
		"skills/lessons/d.md": f("# D"),
		dir + "question.yaml": f("title: Q\ndomain: d\ntask: do\nhints: [h]\nchecks:\n" +
			"  - {id: main, title: T, type: exec, run: echo ok, explain: e}\n"),
		dir + "setup.sh":    f("echo setup"),
		dir + "cleanup.sh":  f("echo cleanup"),
		dir + "solution.sh": f("echo solution"),
		dir + "solution.md": f("do it"),
	}
}

// The reason of a recommendation is worded by the kind of the track: a skills
// track has exercises and a skills assessment, never questions and an exam.
func TestRecommendationReasonFollowsTheTrackKind(t *testing.T) {
	a := newQuizAPI(t, skillsTrack())
	reasons := func(track string) []string {
		var p struct{ Recommendations []struct{ Reason string } }
		if err := json.Unmarshal([]byte(a.ok("GET", "/api/tracks/"+track+"/progress", "")), &p); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range p.Recommendations {
			out = append(out, r.Reason)
		}
		return out
	}
	if got := reasons("skills"); len(got) != 1 || got[0] != "Not practised yet · worth 100% of the skills assessment" {
		t.Errorf("skills track, not practised: %q", got)
	}
	if got := reasons("quiz"); len(got) != 2 || got[0] != "Not practised yet · worth 50% of the exam" {
		t.Errorf("theory track, not practised: %q", got)
	}

	// After an attempt the reason counts what was solved, again in the track's words.
	sid := a.session("practice")
	a.ok("POST", "/api/sessions/"+sid+"/questions/a1/check", `{"answer":["`+a.wrong("a1")+`"]}`)
	if got := strings.Join(reasons("quiz"), "\n"); !strings.Contains(got, "0 of 1 questions solved · worth 50% of the exam") {
		t.Errorf("theory track after an attempt: %q", got)
	}
}

// An exam record says how large the assessment was, so that a client can tell
// a quick round from a full one.
func TestProgressExamsCarryTheirSize(t *testing.T) {
	a := newQuizAPI(t, nil)
	var ses struct{ ID string }
	if err := json.Unmarshal([]byte(a.ok("POST", "/api/sessions", `{"mode":"exam","track":"quiz","questions":1,"minutes":3}`)), &ses); err != nil {
		t.Fatal(err)
	}
	a.ok("POST", "/api/sessions/"+ses.ID+"/start", "")
	a.ok("POST", "/api/sessions/"+ses.ID+"/finish", "")
	var p struct {
		Exams []struct {
			SessionID          string
			Questions, Minutes int
		}
	}
	if err := json.Unmarshal([]byte(a.ok("GET", "/api/tracks/quiz/progress", "")), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Exams) != 1 || p.Exams[0].SessionID != ses.ID || p.Exams[0].Questions != 1 || p.Exams[0].Minutes != 3 {
		t.Errorf("exams = %+v, want one of 1 question and 3 minutes", p.Exams)
	}
}
