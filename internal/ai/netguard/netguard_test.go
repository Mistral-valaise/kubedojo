package netguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

// fakeDNS answers from a table and fails for every other name.
type fakeDNS map[string][]string

func (f fakeDNS) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	var out []net.IPAddr
	for _, s := range f[host] {
		out = append(out, net.IPAddr{IP: net.ParseIP(s)})
	}
	if len(out) == 0 {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return out, nil
}

var dns = fakeDNS{
	"api.example.com":   {"93.184.216.34"},
	"gpu.lan":           {"192.168.1.20"},
	"metadata.internal": {"169.254.169.254"},
	"mixed.example.com": {"93.184.216.34", "169.254.169.254"},
	"half.lan":          {"10.0.0.5", "93.184.216.34"},
	"mapped.example":    {"::ffff:169.254.169.254"},
	"cgnat.example":     {"100.64.1.1"},
	"v6.lan":            {"fd00::10"},
}

func TestCheckURL(t *testing.T) {
	p := Policy{SelfAddr: "127.0.0.1:4400", Resolver: dns}
	for _, tc := range []struct {
		raw  string
		ok   bool
		want string // part of the reason when refused
	}{
		{"https://api.openai.com/v1", true, ""}, // not in the table: https to an unresolved name is not a policy matter
		{"https://api.example.com/v1", true, ""},
		{"http://localhost:11434/v1", true, ""},
		{"http://ollama.localhost:11434/v1", true, ""},
		{"http://127.0.0.1:1234/v1", true, ""},
		{"http://[::1]:11434/v1", true, ""},
		{"http://192.168.1.20:8000/v1", true, ""},
		{"http://gpu.lan:8000/v1", true, ""},
		{"http://v6.lan/v1", true, ""},
		{"https://gpu.lan/v1", true, ""},
		{"  https://api.example.com/v1  ", true, ""},

		{"", false, "empty"},
		{"ftp://api.example.com", false, "http:// or https://"},
		{"file:///etc/passwd", false, "http:// or https://"},
		{"api.example.com/v1", false, "http:// or https://"},
		{"https://user:pw@api.example.com/v1", false, "user name or password"},
		{"https://api.example.com/v1#frag", false, "fragment"},
		{"https://api.example.com/v1#", false, "fragment"},
		{"https://api.example.com/v1?key=1", false, "query"},
		{"https:///v1", false, "no host"},
		{"https://api.example.com:99999/v1", false, "port"},
		{"http://api.example.com/v1", false, "use https"},
		{"http://half.lan/v1", false, "use https"},
		{"http://unknown.example/v1", false, "could not be resolved"},
		{"http://169.254.169.254/latest/meta-data", false, "link-local"},
		{"https://169.254.169.254/", false, "link-local"},
		{"https://metadata.internal/", false, "link-local"},
		{"https://mixed.example.com/", false, "link-local"},
		{"https://mapped.example/", false, "link-local"},
		{"http://[::ffff:169.254.169.254]/", false, "link-local"},
		{"http://[fe80::1]/", false, "link-local"},
		{"http://[64:ff9b::a9fe:a9fe]/", false, "link-local"},
		{"http://0.0.0.0:11434/", false, "unspecified"},
		{"http://[::]:11434/", false, "unspecified"},
		{"http://224.0.0.1/", false, "multicast"},
		{"http://100.64.1.1/", false, "carrier-grade"},
		{"https://cgnat.example/", false, "carrier-grade"},
		{"http://255.255.255.255/", false, "broadcast"},
		{"http://127.0.0.1:4400/v1", false, "this kubedojo server"},
		{"http://localhost:4400/v1", false, "this kubedojo server"},
		{"http://[::1]:4400/v1", false, "this kubedojo server"},
	} {
		u, err := p.CheckURL(context.Background(), tc.raw)
		if tc.ok {
			if err != nil || u == nil {
				t.Errorf("%q: refused: %v", tc.raw, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%q: allowed", tc.raw)
			continue
		}
		if !errors.Is(err, ErrBlocked) {
			t.Errorf("%q: %v does not match ErrBlocked", tc.raw, err)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: reason %q does not mention %q", tc.raw, err, tc.want)
		}
	}
}

// The reason of a refusal is shown to the user and logged. It must not quote
// credentials somebody put into the URL.
func TestCheckURLDoesNotEchoCredentials(t *testing.T) {
	for _, raw := range []string{"https://user:hunter2@api.example.com/v1", "https://user:hunter2@%zz/v1", "http://hunter2@[::1/v1"} {
		_, err := Policy{Resolver: dns}.CheckURL(context.Background(), raw)
		if err == nil {
			t.Fatalf("%q: allowed", raw)
		}
		if strings.Contains(err.Error(), "hunter2") {
			t.Errorf("%q: the reason quotes the password: %v", raw, err)
		}
	}
}

func TestCheckIP(t *testing.T) {
	for addr, ok := range map[string]bool{
		"127.0.0.1":              true,
		"10.1.2.3":               true,
		"172.16.0.1":             true,
		"192.168.0.1":            true,
		"93.184.216.34":          true,
		"::1":                    true,
		"fd12::1":                true,
		"2606:4700::1111":        true,
		"::ffff:10.0.0.1":        true,
		"0.0.0.0":                false,
		"::":                     false,
		"169.254.169.254":        false,
		"169.254.0.1":            false,
		"::ffff:169.254.169.254": false,
		"fe80::1":                false,
		"fe80::1%en0":            false,
		"224.0.0.251":            false,
		"ff02::1":                false,
		"100.64.0.1":             false,
		"100.127.255.254":        false,
		"::ffff:100.64.0.1":      false,
		"64:ff9b::a9fe:a9fe":     false, // NAT64 of 169.254.169.254
		"255.255.255.255":        false,
	} {
		err := CheckIP(netip.MustParseAddr(addr))
		if (err == nil) != ok {
			t.Errorf("%s: allowed = %v, want %v (%v)", addr, err == nil, ok, err)
		}
	}
}

// selfPolicy listens on every interface; 192.168.7.7 is one of its addresses.
func selfPolicy(addr string) Policy {
	return Policy{SelfAddr: addr, interfaces: func() []netip.Addr { return []netip.Addr{netip.MustParseAddr("192.168.7.7")} }}
}

func TestSelfAddress(t *testing.T) {
	for _, tc := range []struct {
		self, ip, port string
		want           bool
	}{
		{"127.0.0.1:4400", "127.0.0.1", "4400", true},
		{"127.0.0.1:4400", "127.0.0.2", "4400", true}, // all of 127/8 is this machine
		{"127.0.0.1:4400", "::1", "4400", true},
		{"127.0.0.1:4400", "127.0.0.1", "4401", false},
		{"127.0.0.1:4400", "192.168.7.7", "4400", false},
		{"0.0.0.0:8080", "127.0.0.1", "8080", true},
		{"0.0.0.0:8080", "192.168.7.7", "8080", true},
		{"0.0.0.0:8080", "192.168.7.8", "8080", false},
		{"[::]:8080", "192.168.7.7", "8080", true},
		{":8080", "192.168.7.7", "8080", true},
		{":8080", "::ffff:192.168.7.7", "8080", true},
		{"192.168.7.7:8080", "192.168.7.7", "8080", true},
		{"192.168.7.7:8080", "127.0.0.1", "8080", false},
		{"", "127.0.0.1", "8080", false},
	} {
		if got := selfPolicy(tc.self).self(netip.MustParseAddr(tc.ip), tc.port); got != tc.want {
			t.Errorf("self=%q %s:%s = %v, want %v", tc.self, tc.ip, tc.port, got, tc.want)
		}
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// The client judges the address it dials, not the name in the URL: a name
// that resolves to the metadata address is refused when the connection is
// made, whatever CheckURL said earlier.
func TestClientRefusesWhatTheNameResolvesTo(t *testing.T) {
	p := Policy{Resolver: dns}
	for _, raw := range []string{"https://metadata.internal/", "https://mapped.example/", "https://mixed.example.com/", "http://half.lan/", "http://api.example.com/"} {
		base := mustURL(t, raw)
		_, err := p.Client(base, ClientOptions{}).Get(raw)
		if err == nil {
			t.Errorf("%s: the request was sent", raw)
			continue
		}
		if !errors.Is(err, ErrBlocked) {
			t.Errorf("%s: %v does not match ErrBlocked", raw, err)
		}
	}
}

// The Control function of the dialer sees the real address even when the
// dial function in front of it was bypassed.
func TestClientControlChecksTheDialledAddress(t *testing.T) {
	base := mustURL(t, "https://api.example.com/")
	tr := Policy{}.Client(base, ClientOptions{}).Transport.(*http.Transport)
	_, err := tr.DialContext(context.Background(), "tcp", "169.254.169.254:80")
	if !errors.Is(err, ErrBlocked) {
		t.Errorf("dial 169.254.169.254: %v", err)
	}
	_, err = tr.DialContext(context.Background(), "tcp", "[::ffff:169.254.169.254]:80")
	if !errors.Is(err, ErrBlocked) {
		t.Errorf("dial the IPv4-mapped address: %v", err)
	}
}

func TestClientReachesLoopbackAndRefusesRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("reached"))
	}))
	defer target.Close()
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirecting.Close()

	c := Policy{}.Client(mustURL(t, redirecting.URL), ClientOptions{})
	res, err := c.Get(target.URL)
	if err != nil {
		t.Fatalf("loopback: %v", err)
	}
	res.Body.Close()

	res, err = c.Get(redirecting.URL)
	if err != nil {
		t.Fatalf("redirect: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Errorf("the redirect was followed: status %d", res.StatusCode)
	}
}

func TestClientRefusesItself(t *testing.T) {
	self := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the server called itself")
	}))
	defer self.Close()
	p := Policy{SelfAddr: self.Listener.Addr().String()}
	_, err := p.Client(mustURL(t, self.URL), ClientOptions{}).Get(self.URL)
	if !errors.Is(err, ErrBlocked) {
		t.Errorf("own address: %v", err)
	}
}

// A proxy from the environment would see every request to a local model.
func TestClientUsesNoProxyForLocalTargets(t *testing.T) {
	p := Policy{Resolver: dns}
	for raw, wantProxy := range map[string]bool{
		"http://localhost:11434/v1":  false,
		"https://gpu.lan/v1":         false,
		"https://192.168.1.20/v1":    false,
		"https://api.example.com/v1": true,
	} {
		tr := p.Client(mustURL(t, raw), ClientOptions{}).Transport.(*http.Transport)
		if got := tr.Proxy != nil; got != wantProxy {
			t.Errorf("%s: proxy from the environment = %v, want %v", raw, got, wantProxy)
		}
	}
}

func TestOrigin(t *testing.T) {
	for raw, want := range map[string]string{
		"http://localhost:11434/v1":    "http://localhost:11434",
		"http://LOCALHOST:11434/other": "http://localhost:11434",
		"https://api.openai.com/v1":    "https://api.openai.com:443",
		"https://api.openai.com:443":   "https://api.openai.com:443",
		"http://gpu.lan":               "http://gpu.lan:80",
		"http://[::1]:8000/v1":         "http://[::1]:8000",
		"not a url":                    "",
		"":                             "",
	} {
		if got := OriginOf(raw); got != want {
			t.Errorf("OriginOf(%q) = %q, want %q", raw, got, want)
		}
	}
}
