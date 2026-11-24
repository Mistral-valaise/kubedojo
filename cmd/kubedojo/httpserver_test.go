package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// factor is by how much the test shortens the timeouts of the server.
const factor = 200

// scaled returns the server of newHTTPServer with every timeout divided by
// factor: ten seconds become 50ms. The test runs against the real settings, in
// a time a test may take. A timeout that is added later is scaled as well,
// and if it limits a request, the test below fails.
func scaled(t *testing.T, h http.Handler) (*httptest.Server, *http.Server) {
	t.Helper()
	hs := newHTTPServer("", h)
	hs.ReadTimeout /= factor
	hs.ReadHeaderTimeout /= factor
	hs.WriteTimeout /= factor
	hs.IdleTimeout /= factor
	srv := httptest.NewUnstartedServer(h)
	srv.Config = hs
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, hs
}

// Grading holds a request for minutes, the tutor's answer and the terminal
// are streams. None of the server's timeouts may end a request that is being
// worked on; they are for clients that send nothing.
func TestLongRequestsSurviveServerTimeouts(t *testing.T) {
	real := newHTTPServer("127.0.0.1:0", http.NotFoundHandler())
	if real.ReadHeaderTimeout != 10*time.Second || real.IdleTimeout != 120*time.Second || real.MaxHeaderBytes != 64<<10 {
		t.Errorf("header timeout %v, idle timeout %v, header limit %d", real.ReadHeaderTimeout, real.IdleTimeout, real.MaxHeaderBytes)
	}
	if real.ReadTimeout != 0 || real.WriteTimeout != 0 {
		t.Errorf("ReadTimeout %v, WriteTimeout %v: they would end a check or a stream that takes longer", real.ReadTimeout, real.WriteTimeout)
	}

	// Longer than every timeout of the server: 900ms.
	var longest time.Duration
	for _, d := range []time.Duration{real.ReadTimeout, real.ReadHeaderTimeout, real.WriteTimeout, real.IdleTimeout} {
		longest = max(longest, d/factor*3/2)
	}
	srv, hs := scaled(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slow": // a check: nothing for a long time, then the answer
			body, _ := io.ReadAll(r.Body)
			select {
			case <-time.After(longest):
			case <-r.Context().Done():
				http.Error(w, "cancelled", http.StatusServiceUnavailable)
				return
			}
			fmt.Fprintf(w, "done %s", body)
		case "/stream": // an event stream: a line now and then, for long
			w.Header().Set("Content-Type", "text/event-stream")
			for i := range 4 {
				fmt.Fprintf(w, "data: %d\n\n", i)
				w.(http.Flusher).Flush()
				select {
				case <-time.After(longest / 3):
				case <-r.Context().Done():
					return
				}
			}
			fmt.Fprint(w, "data: end\n\n")
		}
	}))
	client := srv.Client()
	start := time.Now()
	res, err := client.Post(srv.URL+"/slow", "application/json", strings.NewReader(`{"a":1}`))
	if err != nil {
		t.Fatalf("a request that takes %v: %v", longest, err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || string(body) != `done {"a":1}` || time.Since(start) < longest {
		t.Errorf("a request that takes %v: %d %q after %v", longest, res.StatusCode, body, time.Since(start))
	}

	start = time.Now()
	res, err = client.Get(srv.URL + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	body, err = io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || !strings.HasSuffix(string(body), "data: end\n\n") || time.Since(start) < longest {
		t.Errorf("a stream of %v: %q (%v) after %v", longest, body, err, time.Since(start))
	}

	// What the timeouts are for. A client that does not finish its headers:
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	start = time.Now()
	fmt.Fprint(conn, "GET /slow HTTP/1.1\r\nHost: localhost\r\n")
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	rest, _ := io.ReadAll(conn) // until the server closes the connection
	if took := time.Since(start); took > 2*time.Second || took < hs.ReadHeaderTimeout/2 {
		t.Errorf("unfinished headers: disconnected after %v (%q), want after about %v", took, rest, hs.ReadHeaderTimeout)
	}

	// A connection nobody uses after its request:
	idle, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer idle.Close()
	fmt.Fprint(idle, "GET /none HTTP/1.1\r\nHost: localhost\r\n\r\n")
	rd := bufio.NewReader(idle)
	if res, err := http.ReadResponse(rd, nil); err != nil {
		t.Fatal(err)
	} else {
		res.Body.Close()
	}
	start = time.Now()
	_ = idle.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := rd.ReadByte(); err != io.EOF {
		t.Errorf("an idle connection: %v, want it closed by the server", err)
	}
	if took := time.Since(start); took > 3*time.Second || took < hs.IdleTimeout/2 {
		t.Errorf("an idle connection was closed after %v, want after about %v", took, hs.IdleTimeout)
	}
}
