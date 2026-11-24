package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Mistral-valaise/kubedojo/internal/httpsec"
)

// The API as `kubedojo up` serves it: on loopback, behind the checks of
// package httpsec. These are the two attacks of a web page the user visits
// while kubedojo runs, and the proof that the UI is not in their way.
func TestAPIBehindTheSecurityChain(t *testing.T) {
	f := newFixture(t, 4)
	srv := httptest.NewUnstartedServer(nil)
	addr := srv.Listener.Addr().String() // 127.0.0.1:<port>
	srv.Config.Handler = httpsec.Chain(httpsec.Config{ListenAddr: addr})(f.api.Handler())
	srv.Start()
	defer srv.Close()
	ws := "ws://" + addr + "/api/sessions/" + f.id + "/terminal"

	dial := func(host, origin string) (*websocket.Conn, int) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		opts := &websocket.DialOptions{Host: host, HTTPHeader: http.Header{}}
		if origin != "" {
			opts.HTTPHeader.Set("Origin", origin)
		}
		c, res, err := websocket.Dial(ctx, ws, opts)
		if err != nil {
			if res == nil {
				t.Fatalf("dial: %v", err)
			}
			return nil, res.StatusCode
		}
		return c, res.StatusCode
	}

	// The terminal of the UI: same origin, a WebSocket through all of it.
	c, code := dial("", "http://"+addr)
	if c == nil {
		t.Fatalf("the UI's terminal was refused with %d", code)
	}
	ctx := context.Background()
	if err := c.Write(ctx, websocket.MessageBinary, []byte("id")); err != nil {
		t.Fatal(err)
	}
	if got := read(t, c, 2); got != "id" {
		t.Errorf("the terminal echoed %q", got)
	}
	c.Close(websocket.StatusNormalClosure, "")

	// DNS rebinding: the page's own name resolves to 127.0.0.1, so Origin and
	// Host agree with each other. Only the Host allowlist tells.
	if c, code := dial("evil.example:4400", "http://evil.example:4400"); c != nil || code != http.StatusForbidden {
		t.Errorf("a terminal under a rebound name: status %d, want 403", code)
	}
	// A page of another site opens the WebSocket directly. The upgrade is a
	// GET, which the cross-origin protection lets pass: the terminal's own
	// Origin check is what refuses it.
	if c, code := dial("", "https://evil.example"); c != nil || code != http.StatusForbidden {
		t.Errorf("a terminal from another origin: status %d, want 403", code)
	}

	do := func(method, path, host string, header map[string]string, body string) (int, http.Header, string) {
		req, err := http.NewRequest(method, "http://"+addr+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if host != "" {
			req.Host = host
		}
		for k, v := range header {
			req.Header.Set(k, v)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		return res.StatusCode, res.Header, string(raw)
	}
	const create = `{"mode":"practice","track":"cka"}`
	for _, c := range []struct {
		name, method, path, host string
		header                   map[string]string
		body                     string
		want                     int
	}{
		{"the UI reads", "GET", "/api/config", "", map[string]string{"Sec-Fetch-Site": "same-origin"}, "", 200},
		{"the UI under localhost", "GET", "/api/config", "localhost:4400", nil, "", 200},
		{"the UI creates a session", "POST", "/api/sessions", "",
			map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://" + addr, "Content-Type": "application/json"}, create, 201},
		{"the UI's page", "GET", "/", "", nil, "", 0}, // 200, or 404 when the UI is not built
		{"a rebound name reads", "GET", "/api/config", "evil.example:4400", map[string]string{"Sec-Fetch-Site": "same-origin"}, "", 403},
		{"a rebound name creates a session", "POST", "/api/sessions", "evil.example:4400",
			map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://evil.example:4400", "Content-Type": "application/json"}, create, 403},
		{"a form of another site, as text/plain", "POST", "/api/sessions", "",
			map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example", "Content-Type": "text/plain"}, create, 403},
		{"a form of another site without a body", "POST", "/api/sessions/" + f.id + "/finish", "",
			map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example", "Content-Type": "application/x-www-form-urlencoded"}, "", 403},
		{"a form of another site, from a browser that marks nothing", "POST", "/api/sessions", "",
			map[string]string{"Origin": "https://evil.example", "Content-Type": "text/plain"}, create, 403},
		// What is left when neither header tells: the body is not read.
		{"text/plain from nowhere", "POST", "/api/sessions", "", map[string]string{"Content-Type": "text/plain"}, create, 415},
		{"a probe with the Pod's address", "GET", "/api/health", "10.42.0.17:4400", nil, "", 200},
	} {
		code, header, raw := do(c.method, c.path, c.host, c.header, c.body)
		if c.want != 0 && code != c.want {
			t.Errorf("%s: %d %s, want %d", c.name, code, raw, c.want)
		}
		if csp := header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "evil") {
			t.Errorf("%s: Content-Security-Policy = %q", c.name, csp)
		}
		if header.Get("X-Content-Type-Options") != "nosniff" || header.Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: the answer lacks the security headers: %v", c.name, header)
		}
	}
}
