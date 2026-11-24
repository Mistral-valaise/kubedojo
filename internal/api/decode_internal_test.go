package api

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/Mistral-valaise/kubedojo/internal/httpsec"
)

// decodeServer serves one handler that reads a JSON body with decode, then
// works for a while, as grading does, and answers 200. bodyTimeout is
// shortened to timeout for as long as the server lives.
func decodeServer(t *testing.T, timeout, work time.Duration, onReject func(string)) (srv *httptest.Server, cancelled *atomic.Int32) {
	t.Helper()
	old := bodyTimeout
	bodyTimeout = timeout
	s, cancelled := &Server{OnReject: onReject}, &atomic.Int32{}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		if !s.decode(w, r, &v, 1<<10) {
			return
		}
		select {
		case <-time.After(work):
		case <-r.Context().Done():
			cancelled.Add(1)
		}
		writeJSON(w, 200, map[string]any{"got": v})
	}))
	t.Cleanup(func() {
		srv.Close()
		bodyTimeout = old
	})
	return srv, cancelled
}

// raw sends request bytes over a connection of its own and returns the status
// line of the answer, how long it took, and whether the server closed the
// connection after it.
func raw(t *testing.T, srv *httptest.Server, request string) (status string, took time.Duration, closed bool) {
	t.Helper()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	start := time.Now()
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	rd := bufio.NewReader(conn)
	res, err := http.ReadResponse(rd, nil)
	if err != nil {
		t.Fatalf("no answer after %v: %v", time.Since(start), err)
	}
	took = time.Since(start)
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, err = rd.ReadByte()
	return res.Status, took, err == io.EOF
}

// A client that does not finish its body is answered and disconnected when
// the time for the body is over. Without the deadline the request would wait
// for as long as the client likes: the server has no read timeout.
func TestSlowBodyIsCutOff(t *testing.T) {
	srv, _ := decodeServer(t, 200*time.Millisecond, 0, nil)
	const head = "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\n"
	for name, c := range map[string]struct{ request, want string }{
		"half a JSON value":             {head + "Content-Length: 50\r\n\r\n{\"a\":", "408 Request Timeout"},
		"nothing of the body":           {head + "Content-Length: 50\r\n\r\n", "408 Request Timeout"},
		"a value, and the rest missing": {head + "Content-Length: 50\r\n\r\n{\"a\":1}", "408 Request Timeout"},
		"half a chunked body":           {head + "Transfer-Encoding: chunked\r\n\r\n5\r\n{\"a\":\r\n", "408 Request Timeout"},
		// Not JSON, so nothing reads the body. The server still waits for it
		// before it answers, and that wait ends as well.
		"a body that is not JSON and does not arrive": {"POST / HTTP/1.1\r\nHost: localhost\r\nContent-Type: text/plain\r\nContent-Length: 50\r\n\r\n",
			"415 Unsupported Media Type"},
	} {
		t.Run(name, func(t *testing.T) {
			status, took, closed := raw(t, srv, c.request)
			if status != c.want {
				t.Errorf("status = %q, want %q", status, c.want)
			}
			if took < 150*time.Millisecond || took > 3*time.Second {
				t.Errorf("answered after %v, want about the body timeout of 200ms", took)
			}
			if !closed {
				t.Error("the connection stayed open: the rest of the body would be read as the next request")
			}
		})
	}
}

// The deadline is for the body alone. A handler that works for longer than
// the deadline after it has read the body is not interrupted: its context is
// not cancelled, and the connection serves the next request.
func TestHandlerOutlivesTheBodyDeadline(t *testing.T) {
	srv, cancelled := decodeServer(t, 100*time.Millisecond, 500*time.Millisecond, nil)
	client := srv.Client()
	for i, c := range []struct{ name, contentType, body string }{
		{"with a body", "application/json", `{"a":1}`},
		{"without a body", "", ""},
		{"with a body again, on the same connection", "application/json; charset=utf-8", `{"a":2}   `},
	} {
		req, err := http.NewRequest("POST", srv.URL, strings.NewReader(c.body))
		if err != nil {
			t.Fatal(err)
		}
		if c.contentType != "" {
			req.Header.Set("Content-Type", c.contentType)
		}
		start := time.Now()
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		out, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || time.Since(start) < 500*time.Millisecond {
			t.Errorf("%s: %d %s after %v, want 200 after the handler's 500ms", c.name, res.StatusCode, out, time.Since(start))
		}
		if n := cancelled.Load(); n != 0 {
			t.Fatalf("%s (request %d): the request context was cancelled while the handler worked", c.name, i+1)
		}
	}
}

// A chunked request has no Content-Length: whether it has a body shows only
// when it is read.
func TestChunkedBodies(t *testing.T) {
	var (
		mu      sync.Mutex
		reasons []string
	)
	srv, _ := decodeServer(t, time.Second, 0, func(r string) {
		mu.Lock()
		reasons = append(reasons, r)
		mu.Unlock()
	})
	const head = "POST / HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding: chunked\r\n"
	for _, c := range []struct{ name, request, want string }{
		{"empty, no Content-Type", head + "\r\n0\r\n\r\n", "200 OK"},
		{"empty, declared as text", head + "Content-Type: text/plain\r\n\r\n0\r\n\r\n", "200 OK"},
		{"JSON", head + "Content-Type: application/json\r\n\r\n7\r\n{\"a\":1}\r\n0\r\n\r\n", "200 OK"},
		{"JSON in two chunks", head + "Content-Type: application/json\r\n\r\n3\r\n{\"a\r\n4\r\n\":1}\r\n0\r\n\r\n", "200 OK"},
		{"JSON declared as text", head + "Content-Type: text/plain\r\n\r\n7\r\n{\"a\":1}\r\n0\r\n\r\n", "415 Unsupported Media Type"},
		{"JSON without a Content-Type", head + "\r\n7\r\n{\"a\":1}\r\n0\r\n\r\n", "415 Unsupported Media Type"},
	} {
		if status, _, _ := raw(t, srv, c.request); status != c.want {
			t.Errorf("%s: status = %q, want %q", c.name, status, c.want)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reasons) != 2 || reasons[0] != httpsec.ReasonContentType || reasons[1] != httpsec.ReasonContentType {
		t.Errorf("OnReject saw %v, want content_type for the two rejected requests", reasons)
	}
}

// A read deadline that passes while a handler works cancels the request's
// context once nothing of the body is left: the server then watches the
// connection, and takes the timeout for a client that went away. So the
// deadline must be gone when the body is read, also when somebody else read
// it first.
func TestBodyDeadlineIsTakenBack(t *testing.T) {
	old := bodyTimeout
	bodyTimeout = 100 * time.Millisecond
	s, cancelled := &Server{}, &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		var v map[string]any
		if !s.decode(w, r, &v, 1<<10) {
			return
		}
		select {
		case <-time.After(400 * time.Millisecond):
		case <-r.Context().Done():
			cancelled.Add(1)
		}
		writeJSON(w, 200, v)
	}))
	defer func() {
		srv.Close()
		bodyTimeout = old
	}()
	res, err := srv.Client().Post(srv.URL, "application/json", strings.NewReader(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 || cancelled.Load() != 0 {
		t.Errorf("status %d, context cancelled %d times; want 200 and a handler that was left alone", res.StatusCode, cancelled.Load())
	}
}

// With telemetry on, the handlers get the response writer of otelhttp, not
// the server's. The deadline for the body must reach the connection through it.
func TestSlowBodyIsCutOffBehindTelemetry(t *testing.T) {
	old := bodyTimeout
	bodyTimeout = 200 * time.Millisecond
	s := &Server{}
	srv := httptest.NewServer(otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		if s.decode(w, r, &v, 1<<10) {
			writeJSON(w, 200, v)
		}
	}), "http"))
	defer func() {
		srv.Close()
		bodyTimeout = old
	}()
	status, took, closed := raw(t, srv, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\nContent-Length: 50\r\n\r\n{\"a\":")
	if status != "408 Request Timeout" || took > 3*time.Second || !closed {
		t.Errorf("status %q after %v, connection closed = %v; want 408 after about 200ms and a closed connection", status, took, closed)
	}
}
