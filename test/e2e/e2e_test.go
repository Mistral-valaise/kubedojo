// Package e2e drives a deployed kubedojo the way a learner's browser does:
// over HTTP and the terminal WebSocket, through whatever sits in front of it.
// It proves hosted mode on a real cluster; see docs/deployment.md.
//
// The tests only run against a server they are pointed at:
//
//	KUBEDOJO_E2E_URL=http://127.0.0.1:8080 KUBEDOJO_E2E_HOST=kubedojo.example.com \
//	  go test -count=1 -timeout 30m -v ./test/e2e/
//
// KUBEDOJO_E2E_HOST is the Host header, for an ingress that is reached by IP.
// KUBEDOJO_E2E_IDLE is how long the terminal stays idle (default 95s).
// KUBEDOJO_E2E_CLUSTER_TRACK names a track with clusters to start (default cka; "-" skips it).
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type client struct {
	t    *testing.T
	base string
	host string
	http *http.Client
}

func newClient(t *testing.T) *client {
	t.Helper()
	base := strings.TrimRight(os.Getenv("KUBEDOJO_E2E_URL"), "/")
	if base == "" {
		t.Skip("set KUBEDOJO_E2E_URL to a deployed kubedojo")
	}
	return &client{t: t, base: base, host: os.Getenv("KUBEDOJO_E2E_HOST"), http: &http.Client{Timeout: 60 * time.Second}}
}

// do sends a request and decodes a JSON answer into out, if out is not nil.
func (c *client) do(method, path string, body, out any) int {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		c.t.Fatal(err)
	}
	if c.host != "" {
		req.Host = c.host
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("%s %s: HTTP %d, not JSON: %.200s", method, path, res.StatusCode, raw)
		}
	}
	return res.StatusCode
}

type session struct {
	ID        string            `json:"id"`
	Status    string            `json:"status"`
	Error     string            `json:"error"`
	Envs      map[string]string `json:"envs"`
	Questions []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Error  string `json:"error"`
	} `json:"questions"`
	Log []struct {
		Msg string `json:"msg"`
	} `json:"log"`
}

type result struct {
	Score  int  `json:"score"`
	Max    int  `json:"max"`
	Passed bool `json:"passed"`
}

// practice starts a practice session of a track, prepares its first question
// and waits until it is ready. It ends the session when the test is over.
func (c *client) practice(track string, wait time.Duration) (session, string) {
	c.t.Helper()
	var tr struct {
		Questions []struct {
			ID string `json:"id"`
		} `json:"questions"`
	}
	if code := c.do("GET", "/api/tracks/"+track, nil, &tr); code != 200 || len(tr.Questions) == 0 {
		c.t.Fatalf("track %s: HTTP %d, %d questions", track, code, len(tr.Questions))
	}
	q := tr.Questions[0].ID
	var s session
	if code := c.do("POST", "/api/sessions", map[string]string{"track": track, "mode": "practice"}, &s); code != 200 && code != 201 {
		c.t.Fatalf("create a %s session: HTTP %d", track, code)
	}
	c.t.Cleanup(func() { c.do("DELETE", "/api/sessions/"+s.ID, nil, nil) })
	start := time.Now()
	if code := c.do("POST", "/api/sessions/"+s.ID+"/questions/"+q+"/prepare", nil, nil); code != 202 && code != 200 {
		c.t.Fatalf("prepare %s: HTTP %d", q, code)
	}
	for {
		c.do("GET", "/api/sessions/"+s.ID, nil, &s)
		if len(s.Questions) > 0 {
			switch s.Questions[0].Status {
			case "ready":
				c.t.Logf("%s: question %s ready after %s", track, q, time.Since(start).Round(time.Second))
				return s, q
			case "error":
				c.t.Fatalf("%s: question %s failed: %s\nprogress log: %v", track, q, s.Questions[0].Error, s.Log)
			}
		}
		if time.Since(start) > wait {
			c.t.Fatalf("%s: question %s not ready after %s; progress log: %v", track, q, wait, s.Log)
		}
		time.Sleep(3 * time.Second)
	}
}

// terminal is a learner's terminal: a WebSocket to the sandbox. It reads all
// the time, as a browser does: reading is also what answers the server's
// pings, without which the server closes the connection.
type terminal struct {
	t    *testing.T
	conn *websocket.Conn
	mu   sync.Mutex
	buf  bytes.Buffer
	err  error
}

func (c *client) terminal(sid string) *terminal {
	c.t.Helper()
	url := "ws" + strings.TrimPrefix(c.base, "http") + "/api/sessions/" + sid + "/terminal?cols=120&rows=40"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, res, err := websocket.Dial(ctx, url, &websocket.DialOptions{Host: c.host})
	if err != nil {
		code := 0
		if res != nil {
			code = res.StatusCode
		}
		c.t.Fatalf("open the terminal: %v (HTTP %d)", err, code)
	}
	conn.SetReadLimit(1 << 22)
	tm := &terminal{t: c.t, conn: conn}
	go func() {
		for {
			_, data, err := conn.Read(context.Background())
			tm.mu.Lock()
			if err != nil {
				tm.err = err
				tm.mu.Unlock()
				return
			}
			tm.buf.Write(data)
			tm.mu.Unlock()
		}
	}()
	c.t.Cleanup(func() { conn.CloseNow() })
	return tm
}

// closed reports why the connection ended, or nil while it is open.
func (tm *terminal) closed() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.err
}

// run types a command and returns when the terminal printed done.
func (tm *terminal) run(command, done string, wait time.Duration) string {
	tm.t.Helper()
	tm.mu.Lock()
	tm.buf.Reset()
	tm.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tm.conn.Write(ctx, websocket.MessageBinary, []byte(command+"\n")); err != nil {
		tm.t.Fatalf("type into the terminal: %v", err)
	}
	for deadline := time.Now().Add(wait); ; time.Sleep(100 * time.Millisecond) {
		tm.mu.Lock()
		out, err := tm.buf.String(), tm.err
		tm.mu.Unlock()
		if strings.Contains(out, done) {
			return out
		}
		if err != nil || time.Now().After(deadline) {
			tm.t.Fatalf("the terminal did not print %q (connection: %v)\nit printed: %s", done, err, out)
		}
	}
}

// marker makes a string the terminal prints only as the output of a command,
// never as the echo of what was typed.
func marker(tag string) (command, printed string) {
	n := time.Now().UnixNano() % 1000000
	return fmt.Sprintf("echo %s-$((%d+1))", tag, n), fmt.Sprintf("%s-%d", tag, n+1)
}

// A learner on a track without a cluster: the sandbox starts, the terminal
// works through the proxy and survives being idle, the exercise is solved in
// the terminal and graded, and a reset makes it unsolved again.
func TestLinuxExerciseThroughTheTerminal(t *testing.T) {
	c := newClient(t)
	if code := c.do("GET", "/api/ready", nil, nil); code != 200 {
		t.Fatalf("/api/ready: HTTP %d", code)
	}
	s, q := c.practice("linux", 5*time.Minute)
	host := s.Envs["base"]
	if host == "" {
		t.Fatalf("the session has no Linux host: %+v", s.Envs)
	}

	// Unsolved, the exercise fails.
	var res result
	if code := c.do("POST", "/api/sessions/"+s.ID+"/questions/"+q+"/check", nil, &res); code != 200 || res.Passed || res.Max == 0 {
		t.Fatalf("check before solving: HTTP %d, %+v", code, res)
	}

	tm := c.terminal(s.ID)
	cmd, out := marker("kd-first")
	tm.run(cmd, out, time.Minute)

	// Idle. Only the server's pings travel; a proxy that counts them as
	// silence cuts the connection here.
	idle := 95 * time.Second
	if v := os.Getenv("KUBEDOJO_E2E_IDLE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatal(err)
		}
		idle = d
	}
	t.Logf("the terminal stays idle for %s", idle)
	time.Sleep(idle)
	if err := tm.closed(); err != nil {
		t.Fatalf("the terminal was closed while it was idle for %s: %v", idle, err)
	}
	cmd, out = marker("kd-after-idle")
	tm.run(cmd, out, time.Minute)

	// Solve it the way a learner does: on the host, with a pipeline. The
	// script travels in a quoted here-document, so that the terminal's own
	// shell leaves $9 alone.
	solution := "cd /opt/course/access-log-report\n" +
		"awk '$9 >= 500 && $9 <= 599 {print $7}' access.log | sort | uniq -c | sort -rn > errors.txt\n" +
		"awk '$9 == 200 {sum += $10} END {print sum}' access.log > bytes.txt"
	cmd, out = marker("kd-solved")
	tm.run(fmt.Sprintf("ssh %s 'bash -s' <<'KDEOF'\n%s\nKDEOF\n%s", host, solution, cmd), out, 2*time.Minute)
	if code := c.do("POST", "/api/sessions/"+s.ID+"/questions/"+q+"/check", nil, &res); code != 200 || !res.Passed || res.Score != res.Max {
		t.Fatalf("check after solving in the terminal: HTTP %d, %+v", code, res)
	}

	// A second terminal: the session survives a closed tab.
	tm.conn.Close(websocket.StatusNormalClosure, "")
	again := c.terminal(s.ID)
	cmd, out = marker("kd-reconnected")
	again.run(cmd, out, time.Minute)

	// Reset: the exercise is as it was at the start.
	if code := c.do("POST", "/api/sessions/"+s.ID+"/questions/"+q+"/prepare?reset=1", nil, nil); code != 202 && code != 200 {
		t.Fatalf("reset: HTTP %d", code)
	}
	for deadline := time.Now().Add(3 * time.Minute); ; time.Sleep(2 * time.Second) {
		c.do("GET", "/api/sessions/"+s.ID, nil, &s)
		if len(s.Questions) > 0 && s.Questions[0].Status == "ready" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the exercise was not ready again after the reset: %+v", s.Questions)
		}
	}
	if code := c.do("POST", "/api/sessions/"+s.ID+"/questions/"+q+"/check", nil, &res); code != 200 || res.Passed {
		t.Fatalf("check after the reset: HTTP %d, %+v; want it unsolved again", code, res)
	}
}

// A Kubernetes exercise: a cluster is created inside the sandbox, kubectl
// works in the terminal and the question is graded.
func TestClusterExercise(t *testing.T) {
	c := newClient(t)
	track := os.Getenv("KUBEDOJO_E2E_CLUSTER_TRACK")
	if track == "-" {
		t.Skip("KUBEDOJO_E2E_CLUSTER_TRACK=-")
	}
	if track == "" {
		track = "cka"
	}
	s, q := c.practice(track, 12*time.Minute)
	tm := c.terminal(s.ID)
	// The nodes of the session's cluster answer.
	out := tm.run("kubectl get nodes --no-headers | grep -c ' Ready' | sed 's/^/kd-nodes-/'", "kd-nodes-", time.Minute)
	if strings.Contains(out, "kd-nodes-0") {
		t.Fatalf("no node of the cluster is Ready:\n%s", out)
	}
	var res result
	if code := c.do("POST", "/api/sessions/"+s.ID+"/questions/"+q+"/check", nil, &res); code != 200 || res.Max == 0 {
		t.Fatalf("check: HTTP %d, %+v", code, res)
	}
	t.Logf("%s: %s graded %d/%d unsolved", track, q, res.Score, res.Max)
}

// A full, timed exam: provisioned, started, finished and graded into a report.
func TestTimedExam(t *testing.T) {
	c := newClient(t)
	var s session
	if code := c.do("POST", "/api/sessions", map[string]any{"track": "linux", "mode": "exam", "questions": 3, "minutes": 5}, &s); code != 200 && code != 201 {
		t.Fatalf("create an exam: HTTP %d", code)
	}
	t.Cleanup(func() { c.do("DELETE", "/api/sessions/"+s.ID, nil, nil) })
	wait := func(want string, limit time.Duration) {
		t.Helper()
		for deadline := time.Now().Add(limit); ; time.Sleep(2 * time.Second) {
			c.do("GET", "/api/sessions/"+s.ID, nil, &s)
			if s.Status == want {
				return
			}
			if s.Status == "error" || time.Now().After(deadline) {
				t.Fatalf("the exam is %q (%s), want %q; log: %v", s.Status, s.Error, want, s.Log)
			}
		}
	}
	wait("ready", 8*time.Minute)
	if code := c.do("POST", "/api/sessions/"+s.ID+"/start", nil, nil); code != 200 && code != 204 {
		t.Fatalf("start: HTTP %d", code)
	}
	wait("running", time.Minute)
	if code := c.do("POST", "/api/sessions/"+s.ID+"/finish", nil, nil); code != 200 && code != 202 && code != 204 {
		t.Fatalf("finish: HTTP %d", code)
	}
	wait("finished", 5*time.Minute)
	var done struct {
		Report *struct {
			Max     int     `json:"max"`
			Percent float64 `json:"percent"`
			Passed  bool    `json:"passed"`
		} `json:"report"`
	}
	c.do("GET", "/api/sessions/"+s.ID, nil, &done)
	if done.Report == nil || done.Report.Max == 0 || done.Report.Passed {
		t.Fatalf("report of an unanswered exam: %+v", done.Report)
	}
}
