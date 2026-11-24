package main

import (
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestAllowedHostsFlag(t *testing.T) {
	t.Setenv("KUBEDOJO_ALLOWED_HOSTS", "from.env.example")
	for name, c := range map[string]struct {
		args []string
		want []string
	}{
		"the environment":          {nil, []string{"from.env.example"}},
		"the flag wins":            {[]string{"--allowed-hosts", "a.example"}, []string{"a.example"}},
		"a list with blanks":       {[]string{"--allowed-hosts", " a.example, *.b.example ,,"}, []string{"a.example", "*.b.example"}},
		"an empty flag is no list": {[]string{"--allowed-hosts", ""}, nil},
	} {
		fl := flag.NewFlagSet("serve", flag.ContinueOnError)
		sec := registerSecFlags(fl)
		if err := fl.Parse(c.args); err != nil {
			t.Fatal(err)
		}
		if got := sec.hosts(); !slices.Equal(got, c.want) {
			t.Errorf("%s: hosts = %q, want %q", name, got, c.want)
		}
	}
}

// wrap is what cmdServe puts around everything: how the server runs decides
// what is checked.
func TestSecurityWrap(t *testing.T) {
	t.Setenv("KUBEDOJO_ALLOWED_HOSTS", "")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	status := func(args []string, p serverParts, method, host string, header map[string]string) (int, http.Header) {
		t.Helper()
		fl := flag.NewFlagSet("serve", flag.ContinueOnError)
		sec := registerSecFlags(fl)
		if err := fl.Parse(args); err != nil {
			t.Fatal(err)
		}
		p.log = log
		r := httptest.NewRequest(method, "/api/sessions", nil)
		r.Host = host
		for k, v := range header {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		sec.wrap(inner, p).ServeHTTP(w, r)
		return w.Code, w.Header()
	}
	local := serverParts{addr: "127.0.0.1:4400", local: true}
	hosted := serverParts{addr: "[::]:4400"}
	chart := []string{"--allowed-hosts", "kubedojo.example.com,kubedojo,kubedojo.kubedojo.svc"}
	cross := map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}
	for _, c := range []struct {
		name   string
		args   []string
		parts  serverParts
		method string
		host   string
		header map[string]string
		want   int
	}{
		{"up: the browser", nil, local, "GET", "127.0.0.1:4400", nil, 204},
		{"up: a rebound name", nil, local, "GET", "evil.example:4400", nil, 403},
		{"up: a form of another site", nil, local, "POST", "127.0.0.1:4400", cross, 403},
		{"up: the dev server's origin", nil, serverParts{addr: "127.0.0.1:4400", devOrigin: "localhost:5173"}, "POST", "localhost:5173",
			map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://localhost:5173"}, 204},
		{"a container without a list", nil, hosted, "GET", "anything.example", nil, 204},
		{"a container without a list: a form of another site", nil, hosted, "POST", "anything.example", cross, 403},
		{"the chart: the Ingress", chart, hosted, "GET", "kubedojo.example.com", nil, 204},
		{"the chart: the Service", chart, hosted, "GET", "kubedojo.kubedojo.svc:80", nil, 204},
		{"the chart: a port-forward", chart, hosted, "GET", "localhost:4400", nil, 204},
		{"the chart: another name", chart, hosted, "GET", "other.example.com", nil, 403},
		{"the chart with OIDC: the proxy passes the Ingress host on", chart, serverParts{addr: "127.0.0.1:4400"}, "GET", "kubedojo.example.com", nil, 204},
		{"the chart with OIDC: a proxy that does not", chart, serverParts{addr: "127.0.0.1:4400"}, "GET", "127.0.0.1:4400", nil, 204},
	} {
		code, header := status(c.args, c.parts, c.method, c.host, c.header)
		if code != c.want {
			t.Errorf("%s: status %d, want %d", c.name, code, c.want)
		}
		if header.Get("Content-Security-Policy") == "" || header.Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: the answer lacks the security headers: %v", c.name, header)
		}
	}
}
