package api_test

import (
	"io"
	"net/http"
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

// send is call with a Content-Type of the caller's choice.
func (a *quizAPI) send(method, path, contentType, body string) (int, string) {
	a.t.Helper()
	req, err := http.NewRequest(method, a.url+path, strings.NewReader(body))
	if err != nil {
		a.t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(raw)
}

// A form on any web page can post text/plain to this server, and the browser
// sends it without asking the server first. Such a body is not read, even
// when it is JSON: a request body must be declared as JSON.
func TestRequestBodiesMustBeDeclaredAsJSON(t *testing.T) {
	a := newQuizAPI(t, nil)
	sid := a.session("practice")
	a.question(sid, "a1")
	q := "/api/sessions/" + sid + "/questions/a1"
	const create = `{"mode":"practice","track":"quiz"}`

	for contentType, want := range map[string]int{
		"application/json":                  201,
		"application/json; charset=utf-8":   201,
		"application/json;charset=UTF-8":    201,
		"Application/JSON":                  201,
		"":                                  415,
		"text/plain":                        415,
		"text/plain;charset=UTF-8":          415,
		"application/x-www-form-urlencoded": 415,
		"multipart/form-data; boundary=x":   415,
		"text/json":                         415,
		"application/json, text/plain":      415,
		"text/plain; application/json":      415,
		"application/jsonp":                 415,
	} {
		code, raw := a.send("POST", "/api/sessions", contentType, create)
		if code != want {
			t.Errorf("POST /api/sessions as %q = %d %s, want %d", contentType, code, raw, want)
		}
		if want == 415 && !strings.Contains(raw, `"error"`) || want == 415 && !strings.Contains(raw, "application/json") {
			t.Errorf("POST /api/sessions as %q: the error does not say what to send: %s", contentType, raw)
		}
	}

	// Every endpoint that reads a body, also the one with an error message of
	// its own. What is rejected changes nothing: the accepted note stays.
	for _, c := range []struct{ method, path, rejected, accepted string }{
		{"POST", "/api/sessions", create, create},
		{"PATCH", q, `{"note":"overwritten"}`, `{"note":"kept"}`},
		{"POST", "/api/tracks/quiz/questions/a1/rating", `{"stars":5}`, `{"stars":5}`},
		{"POST", q + "/check", `{"answer":["` + a.correct("a1") + `"]}`, `{"answer":["` + a.correct("a1") + `"]}`},
	} {
		if code, raw := a.send(c.method, c.path, "application/json", c.accepted); code < 200 || code > 299 {
			t.Errorf("%s %s as application/json = %d %s, want success", c.method, c.path, code, raw)
		}
		if code, raw := a.send(c.method, c.path, "text/plain", c.rejected); code != 415 {
			t.Errorf("%s %s as text/plain = %d %s, want 415", c.method, c.path, code, raw)
		}
	}
	if raw := a.ok("GET", q, ""); !strings.Contains(raw, `"note":"kept"`) {
		t.Errorf("a body sent as text/plain changed the question: %s", raw)
	}

	// A request without a body needs no Content-Type and may carry any: the UI
	// starts, finishes, prepares and grades without one.
	for _, contentType := range []string{"", "text/plain", "application/x-www-form-urlencoded"} {
		if code, raw := a.send("POST", q+"/prepare?reset=1", contentType, ""); code != 202 {
			t.Errorf("resetting without a body, as %q = %d %s, want 202", contentType, code, raw)
		}
		if code, raw := a.send("POST", q+"/check", contentType, ""); code != 200 {
			t.Errorf("grading without a body, as %q = %d %s, want 200", contentType, code, raw)
		}
	}
}
