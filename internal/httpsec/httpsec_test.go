package httpsec

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var ok = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })

// serve sends one request through h and returns the answer.
func serve(h http.Handler, method, path, host string, header map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.Host = host
	for k, v := range header {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func errorOf(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error == "" {
		t.Fatalf("the body is not {\"error\": ...}: %q (%v)", w.Body.String(), err)
	}
	return body.Error
}

func TestHostname(t *testing.T) {
	for in, want := range map[string]string{
		"localhost":         "localhost",
		"localhost:4400":    "localhost",
		"LOCALHOST:4400":    "localhost",
		"127.0.0.1:4400":    "127.0.0.1",
		"[::1]:4400":        "::1",
		"[::1]":             "::1",
		"::1":               "::1",
		"[::]:4400":         "::",
		"example.com.":      "example.com",
		"Example.COM.:8443": "example.com",
		"":                  "",
		":4400":             "",
	} {
		if got := hostname(in); got != want {
			t.Errorf("hostname(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHostAllowlist(t *testing.T) {
	local := Config{ListenAddr: "127.0.0.1:4400"}
	for _, c := range []struct {
		name string
		cfg  Config
		host string
		want bool
	}{
		// What a local kubedojo is reached under.
		{"localhost", local, "localhost:4400", true},
		{"localhost without a port", local, "localhost", true},
		{"another port", local, "localhost:5173", true},
		{"upper case", local, "LocalHost:4400", true},
		{"a trailing dot", local, "localhost.:4400", true},
		{"IPv4 loopback", local, "127.0.0.1:4400", true},
		{"IPv6 loopback", local, "[::1]:4400", true},
		{"IPv6 loopback without a port", local, "[::1]", true},
		{"a name below .localhost", local, "kubedojo.localhost:4400", true},
		{"a name below .localhost, upper case, with a dot", local, "Kubedojo.LOCALHOST.", true},
		// DNS rebinding: the attacker's name resolves to 127.0.0.1.
		{"a foreign name", local, "evil.example:4400", false},
		{"a foreign name that ends like localhost", local, "evillocalhost:4400", false},
		{"a foreign name that starts like localhost", local, "localhost.evil.example", false},
		{"a foreign name that contains the address", local, "127.0.0.1.evil.example", false},
		{"another address", local, "10.1.2.3:4400", false},
		{"no Host", local, "", false},
		{".localhost alone", local, ".localhost", false},

		// The address the server listens on.
		{"the listen address", Config{ListenAddr: "192.168.1.20:4400"}, "192.168.1.20:4400", true},
		{"the listen address, IPv6", Config{ListenAddr: "[fd00::20]:4400"}, "[FD00::20]:4400", true},
		{"a name for the listen address", Config{ListenAddr: "192.168.1.20:4400"}, "box.lan:4400", false},
		{"localhost on a LAN address", Config{ListenAddr: "192.168.1.20:4400"}, "localhost:4400", true},

		// --allowed-hosts.
		{"a listed host", Config{ListenAddr: "0.0.0.0:4400", AllowedHosts: []string{"kubedojo.example.com"}}, "kubedojo.example.com", true},
		{"a listed host, any port", Config{ListenAddr: "0.0.0.0:4400", AllowedHosts: []string{"kubedojo.example.com"}}, "KUBEDOJO.example.com:8443", true},
		{"a listed host, written with a port", Config{ListenAddr: "0.0.0.0:4400", AllowedHosts: []string{"kubedojo.example.com:443"}}, "kubedojo.example.com", true},
		{"a listed host, written as a URL", Config{ListenAddr: "0.0.0.0:4400", AllowedHosts: []string{"https://kubedojo.example.com/"}}, "kubedojo.example.com", true},
		{"with a list, another host", Config{ListenAddr: "0.0.0.0:4400", AllowedHosts: []string{"kubedojo.example.com"}}, "evil.example", false},
		{"with a list, localhost still", Config{ListenAddr: "0.0.0.0:4400", AllowedHosts: []string{"kubedojo.example.com"}}, "localhost:4400", true},
		{"a listed IPv6 address", local.with("[2001:db8::1]"), "[2001:db8::1]:4400", true},
		{"a wildcard: a subdomain", local.with("*.example.com"), "a.example.com", true},
		{"a wildcard: a deeper subdomain", local.with("*.example.com"), "a.b.Example.com:443", true},
		{"a wildcard: not the domain itself", local.with("*.example.com"), "example.com", false},
		{"a wildcard: not a name that ends alike", local.with("*.example.com"), "evilexample.com", false},
		{"a wildcard in the middle is no entry", local.with("kube*.example.com"), "kubedojo.example.com", false},
		{"blanks around the entries", local.with(" a.example ", "", " b.example"), "b.example", true},
		{"the check turned off", local.with("*"), "evil.example", true},
		{"the check turned off next to other entries", local.with("a.example", "*"), "evil.example", true},

		// --dev-origin.
		{"the dev server", Config{ListenAddr: "127.0.0.1:4400", DevOrigin: "dev.box.lan:5173"}, "dev.box.lan:5173", true},
		{"the dev server with a scheme", Config{ListenAddr: "127.0.0.1:4400", DevOrigin: "http://dev.box.lan:5173"}, "dev.box.lan:5173", true},
		{"a dev origin of * does not turn the check off", Config{ListenAddr: "127.0.0.1:4400", DevOrigin: "*"}, "evil.example", false},

		// All interfaces and no list: nothing to compare with.
		{"all interfaces, no list", Config{ListenAddr: "0.0.0.0:4400"}, "evil.example", true},
		{"all interfaces (IPv6), no list", Config{ListenAddr: "[::]:4400"}, "evil.example", true},
		{"all interfaces (no host), no list", Config{ListenAddr: ":4400"}, "evil.example", true},
		{"all interfaces, an empty list", Config{ListenAddr: "0.0.0.0:4400", AllowedHosts: []string{"", " "}}, "evil.example", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := serve(HostAllowlist(c.cfg)(ok), "GET", "/api/config", c.host, nil)
			if got := w.Code == http.StatusNoContent; got != c.want {
				t.Fatalf("Host %q: status %d, allowed = %v, want %v", c.host, w.Code, got, c.want)
			}
			if !c.want {
				if w.Code != http.StatusForbidden {
					t.Errorf("status = %d, want 403", w.Code)
				}
				// Whoever sees this must learn how to allow the host.
				if msg := errorOf(t, w); !strings.Contains(msg, "--allowed-hosts") || !strings.Contains(msg, "KUBEDOJO_ALLOWED_HOSTS") {
					t.Errorf("the error does not name the setting: %q", msg)
				}
			}
		})
	}
}

func (c Config) with(hosts ...string) Config {
	c.AllowedHosts = hosts
	return c
}

// The kubelet probes with the Pod's IP address as Host.
func TestHostAllowlistExemptsTheProbes(t *testing.T) {
	h := HostAllowlist(Config{ListenAddr: "0.0.0.0:4400", AllowedHosts: []string{"kubedojo.example.com"}})(ok)
	for path, want := range map[string]int{
		"/api/health":       http.StatusNoContent,
		"/api/ready":        http.StatusNoContent,
		"/api/config":       http.StatusForbidden,
		"/api/health/":      http.StatusForbidden,
		"/api/healthz":      http.StatusForbidden,
		"/":                 http.StatusForbidden,
		"/api/ready/../x":   http.StatusForbidden,
		"/api/sessions/abc": http.StatusForbidden,
	} {
		if w := serve(h, "GET", path, "10.42.0.17:4400", nil); w.Code != want {
			t.Errorf("GET %s with the Pod IP as Host = %d, want %d", path, w.Code, want)
		}
	}
}

func TestHostAllowlistReportsAndWarns(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	reasons := map[string]int{}
	h := HostAllowlist(Config{ListenAddr: "127.0.0.1:4400", Log: log, OnReject: func(r string) { reasons[r]++ }})(ok)
	if logs.Len() != 0 {
		t.Errorf("a server on loopback warned at start: %s", logs.String())
	}
	for range 3 {
		serve(h, "GET", "/api/config", "evil.example", nil)
	}
	serve(h, "GET", "/api/config", "localhost:4400", nil)
	if reasons[ReasonHost] != 3 || len(reasons) != 1 {
		t.Errorf("OnReject saw %v, want host 3 times", reasons)
	}
	// One line per host, not one per request.
	if n := strings.Count(logs.String(), "evil.example"); n != 1 {
		t.Errorf("the rejected host was logged %d times, want once:\n%s", n, logs.String())
	}

	for name, c := range map[string]struct {
		cfg  Config
		want string
	}{
		"all interfaces":     {Config{ListenAddr: "0.0.0.0:4400"}, "listening on all interfaces without --allowed-hosts; any Host header is accepted"},
		"the check is off":   {Config{ListenAddr: "127.0.0.1:4400", AllowedHosts: []string{"*"}}, "the Host check is off"},
		"a list was given":   {Config{ListenAddr: "0.0.0.0:4400", AllowedHosts: []string{"a.example"}}, ""},
		"a specific address": {Config{ListenAddr: "192.168.1.20:4400"}, ""},
	} {
		logs.Reset()
		c.cfg.Log = log
		HostAllowlist(c.cfg)
		if c.want == "" && logs.Len() != 0 || c.want != "" && (!strings.Contains(logs.String(), c.want) || !strings.Contains(logs.String(), "level=WARN")) {
			t.Errorf("%s: logged %q, want a warning with %q", name, logs.String(), c.want)
		}
	}
}

func TestCrossOrigin(t *testing.T) {
	const host = "127.0.0.1:4400"
	for _, c := range []struct {
		name   string
		dev    string
		method string
		header map[string]string
		want   int
	}{
		{"the UI itself", "", "POST", map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://" + host}, 204},
		{"typed into the address bar", "", "POST", map[string]string{"Sec-Fetch-Site": "none"}, 204},
		{"not a browser: curl, a Go client", "", "POST", nil, 204},
		{"an older browser, same origin", "", "POST", map[string]string{"Origin": "http://" + host}, 204},
		// A page of another site posts a form to the API.
		{"another site", "", "POST", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, 403},
		{"another port of this machine", "", "POST", map[string]string{"Sec-Fetch-Site": "same-site", "Origin": "http://127.0.0.1:8000"}, 403},
		{"another site, DELETE", "", "DELETE", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, 403},
		{"another site, PATCH", "", "PATCH", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, 403},
		{"an older browser, another site", "", "POST", map[string]string{"Origin": "https://evil.example"}, 403},
		{"a sandboxed frame", "", "POST", map[string]string{"Origin": "null"}, 403},
		// Reading is not protected here: the browser does not hand the answer
		// to another origin. The WebSocket upgrade is a GET as well.
		{"another site, GET", "", "GET", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, 204},
		{"another site, the WebSocket upgrade", "", "GET", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example",
			"Connection": "Upgrade", "Upgrade": "websocket"}, 204},
		// The UI dev server.
		{"the dev server, as a host", "localhost:5173", "POST", map[string]string{"Sec-Fetch-Site": "same-site", "Origin": "http://localhost:5173"}, 204},
		{"the dev server, with its scheme", "http://localhost:5173", "POST", map[string]string{"Sec-Fetch-Site": "same-site", "Origin": "http://localhost:5173"}, 204},
		{"the dev server, another scheme than configured", "http://localhost:5173", "POST", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://localhost:5173"}, 403},
		{"with a dev server, another site still", "localhost:5173", "POST", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, 403},
		{"a dev origin with a wildcard trusts nobody", "*", "POST", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, 403},
	} {
		t.Run(c.name, func(t *testing.T) {
			reasons := map[string]int{}
			h := CrossOrigin(Config{DevOrigin: c.dev, OnReject: func(r string) { reasons[r]++ }})(ok)
			w := serve(h, c.method, "/api/sessions", host, c.header)
			if w.Code != c.want {
				t.Fatalf("status = %d, want %d", w.Code, c.want)
			}
			if c.want == 403 {
				errorOf(t, w)
				if reasons[ReasonCrossOrigin] != 1 {
					t.Errorf("OnReject saw %v, want cross_origin once", reasons)
				}
			} else if len(reasons) != 0 {
				t.Errorf("OnReject saw %v for an accepted request", reasons)
			}
		})
	}
}

func TestHeaders(t *testing.T) {
	w := serve(Headers(ok), "GET", "/", "localhost:4400", nil)
	for name, want := range map[string]string{
		"Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
			"font-src 'self' data:; connect-src 'self' ws://localhost:4400 wss://localhost:4400; " +
			"frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'",
		"X-Content-Type-Options":       "nosniff",
		"Referrer-Policy":              "no-referrer",
		"X-Frame-Options":              "DENY",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
	} {
		if got := w.Header().Get(name); got != want {
			t.Errorf("%s =\n  %q, want\n  %q", name, got, want)
		}
	}
}

// The Host header may hold characters that mean something in a policy. Only a
// plain name or address gets into it.
func TestContentSecurityPolicyHost(t *testing.T) {
	const plain = "connect-src 'self';"
	for host, want := range map[string]string{
		"localhost:4400":            "connect-src 'self' ws://localhost:4400 wss://localhost:4400;",
		"Kubedojo.Example.com":      "connect-src 'self' ws://kubedojo.example.com wss://kubedojo.example.com;",
		"127.0.0.1:4400":            "connect-src 'self' ws://127.0.0.1:4400 wss://127.0.0.1:4400;",
		"[::1]:4400":                plain, // a policy has no IPv6 literals
		"":                          plain,
		"a;script-src":              plain,
		"a;script-src='unsafe-eval": plain,
		"*":                         plain,
		"*.example.com":             plain,
		"a,b":                       plain,
		"a:b":                       plain,
		"a:999999":                  plain,
		"-a":                        plain,
		"a b":                       plain,
	} {
		got := csp(host)
		if !strings.Contains(got, want) {
			t.Errorf("csp(%q) = %q, want it to contain %q", host, got, want)
		}
		if strings.Contains(got, "unsafe-eval") || strings.Count(got, "script-src") != 1 || strings.Contains(got, "*") {
			t.Errorf("csp(%q) = %q: the host changed the policy", host, got)
		}
	}
}

func TestRecover(t *testing.T) {
	var logs bytes.Buffer
	h := Recover(slog.New(slog.NewTextHandler(&logs, nil)))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	w := serve(h, "GET", "/api/config", "localhost", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if msg := errorOf(t, w); msg != "internal error" {
		t.Errorf("error = %q: a panic must not leak into the answer", msg)
	}
	for _, want := range []string{"level=ERROR", "boom", "path=/api/config", "httpsec_test.go"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("the log lacks %q:\n%s", want, logs.String())
		}
	}

	// The handler's own way to drop a connection is not an error.
	logs.Reset()
	abort := Recover(slog.New(slog.NewTextHandler(&logs, nil)))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	func() {
		defer func() {
			if v := recover(); v != http.ErrAbortHandler {
				t.Errorf("recovered %v, want http.ErrAbortHandler passed on", v)
			}
		}()
		serve(abort, "GET", "/", "localhost", nil)
	}()
	if logs.Len() != 0 {
		t.Errorf("http.ErrAbortHandler was logged: %s", logs.String())
	}
}

// The order of the chain: a Host that is not allowed is answered first, and
// every answer carries the headers, the rejections included.
func TestChain(t *testing.T) {
	reasons := map[string]int{}
	reached := 0
	h := Chain(Config{ListenAddr: "127.0.0.1:4400", OnReject: func(r string) { reasons[r]++ }})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reached++
			if r.URL.Path == "/panic" {
				panic("boom")
			}
			w.WriteHeader(http.StatusNoContent)
		}))
	cross := map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}
	for _, c := range []struct {
		name, method, path, host string
		header                   map[string]string
		want, reached            int
		csp                      string
	}{
		{"the UI", "POST", "/api/sessions", "localhost:4400", map[string]string{"Sec-Fetch-Site": "same-origin"}, 204, 1, "ws://localhost:4400"},
		{"DNS rebinding", "POST", "/api/sessions", "evil.example:4400", map[string]string{"Sec-Fetch-Site": "same-origin"}, 403, 0, "connect-src 'self';"},
		{"DNS rebinding, reading", "GET", "/api/sessions", "evil.example:4400", nil, 403, 0, "connect-src 'self';"},
		{"a form of another site", "POST", "/api/sessions", "127.0.0.1:4400", cross, 403, 0, "connect-src 'self';"},
		{"both", "POST", "/api/sessions", "evil.example:4400", cross, 403, 0, "connect-src 'self';"},
		{"a probe", "GET", "/api/ready", "10.42.0.17:4400", nil, 204, 1, "ws://10.42.0.17:4400"},
		{"a panic", "GET", "/panic", "localhost:4400", nil, 500, 1, "connect-src 'self'"},
	} {
		reached = 0
		w := serve(h, c.method, c.path, c.host, c.header)
		if w.Code != c.want || reached != c.reached {
			t.Errorf("%s: status %d, handler reached %d times; want %d and %d", c.name, w.Code, reached, c.want, c.reached)
		}
		if got := w.Header().Get("Content-Security-Policy"); !strings.Contains(got, c.csp) || strings.Contains(got, "evil") {
			t.Errorf("%s: Content-Security-Policy = %q, want it to contain %q", c.name, got, c.csp)
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: the answer lacks the security headers: %v", c.name, w.Header())
		}
	}
	// "both" is a Host rejection: the Host check comes first.
	if reasons[ReasonHost] != 3 || reasons[ReasonCrossOrigin] != 1 {
		t.Errorf("OnReject saw %v, want host 3 times and cross_origin once", reasons)
	}
}
