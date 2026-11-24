package api_test

import (
	"strings"
	"testing"
)

// padded returns a JSON object of exactly size bytes: fields, and a field the
// handlers do not know that fills the rest.
func padded(t *testing.T, fields string, size int) string {
	t.Helper()
	head, tail := `{`+fields+`,"pad":"`, `"}`
	if size < len(head)+len(tail) {
		t.Fatalf("%d bytes do not hold %s", size, fields)
	}
	return head + strings.Repeat("x", size-len(head)-len(tail)) + tail
}

// Every request body has a size limit. A body of exactly the limit is read, a
// body one byte larger is rejected with the error the endpoint has for a body
// it cannot read.
func TestRequestBodiesAreLimited(t *testing.T) {
	a := newQuizAPI(t, nil)
	sid := a.session("practice")
	a.question(sid, "a1") // opening it prepares it: only then can it be changed
	q := "/api/sessions/" + sid + "/questions/a1"
	for _, c := range []struct {
		name, method, path, fields string
		limit                      int
		wantErr                    string
	}{
		{"create session", "POST", "/api/sessions", `"mode":"practice","track":"quiz"`, 64 << 10, "invalid JSON"},
		{"patch question", "PATCH", q, `"note":"n"`, 16 << 10, "invalid JSON"},
		{"check", "POST", q + "/check", `"answer":["` + a.correct("a1") + `"]`, 8 << 10, "invalid JSON"},
		{"rating", "POST", "/api/tracks/quiz/questions/a1/rating", `"stars":5`, 8 << 10, "stars must be between 1 and 5"},
	} {
		t.Run(c.name, func(t *testing.T) {
			code, raw := a.call(c.method, c.path, padded(t, c.fields, c.limit+1))
			if code != 400 || !strings.Contains(raw, c.wantErr) {
				t.Errorf("a body of %d bytes = %d %s, want 400 %q", c.limit+1, code, raw, c.wantErr)
			}
			if code, raw := a.call(c.method, c.path, padded(t, c.fields, c.limit)); code < 200 || code > 299 {
				t.Errorf("a body of %d bytes = %d %s, want success", c.limit, code, raw)
			}
		})
	}
}

// Which endpoints need a body did not change with the limits: grading a
// question takes none, creating a session and changing a question do.
func TestEmptyRequestBodies(t *testing.T) {
	a := newQuizAPI(t, nil)
	sid := a.session("practice")
	q := "/api/sessions/" + sid + "/questions/a1"
	for _, c := range []struct {
		method, path string
		want         int
	}{
		{"POST", "/api/sessions", 400},
		{"PATCH", q, 400},
		{"POST", "/api/tracks/quiz/questions/a1/rating", 400},
		{"POST", q + "/check", 200},
	} {
		if code, raw := a.call(c.method, c.path, ""); code != c.want {
			t.Errorf("%s %s without a body = %d %s, want %d", c.method, c.path, code, raw, c.want)
		}
	}
}
