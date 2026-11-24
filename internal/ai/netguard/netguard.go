// Package netguard decides which base URLs the assistant may talk to.
//
// The base URL of a model provider is configurable, also through the settings
// API, so it is input: without a policy it would let a caller make the server
// send requests to the cloud metadata service or to itself. The policy has two
// halves. CheckURL judges a URL when it is configured. The HTTP client judges
// again when it connects, on the IP address it actually dials, so that a name
// which resolves differently later cannot get around the first check.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrBlocked is what every refusal of the policy matches with errors.Is.
var ErrBlocked = errors.New("blocked by the base URL policy")

// Error is a refusal of the policy. Its text is safe to show: it names the
// rule and the host, never a credential.
type Error struct {
	Reason string
	// Unresolved is set when a host name could not be looked up, so the
	// policy could not tell whether plain http is allowed for it.
	Unresolved bool
}

func (e *Error) Error() string        { return e.Reason }
func (e *Error) Is(target error) bool { return target == ErrBlocked }

// BlockedURL marks the error for packages that classify failures without
// importing this one.
func (e *Error) BlockedURL() bool { return true }

func blocked(format string, args ...any) *Error { return &Error{Reason: fmt.Sprintf(format, args...)} }

// Resolver looks a host name up. *net.Resolver is one.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// Policy is the base URL policy of one server.
type Policy struct {
	// SelfAddr is the address this server listens on (host:port). Requests to
	// it are refused: an assistant that calls its own server loops.
	SelfAddr string
	// Resolver looks host names up; nil is the system resolver.
	Resolver Resolver
	// interfaces lists the addresses of this machine; nil asks the system.
	interfaces func() []netip.Addr
}

func (p Policy) resolver() Resolver {
	if p.Resolver != nil {
		return p.Resolver
	}
	return net.DefaultResolver
}

var (
	cgnat = netip.MustParsePrefix("100.64.0.0/10")
	nat64 = netip.MustParsePrefix("64:ff9b::/96")
)

// normalize returns the address the packets really go to: the IPv4 address of
// an IPv4-mapped or NAT64 IPv6 address, without a zone.
func normalize(ip netip.Addr) netip.Addr {
	ip = ip.Unmap().WithZone("")
	if ip.Is6() && nat64.Contains(ip) {
		b := ip.As16()
		return netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]})
	}
	return ip
}

// Local reports whether ip is a loopback or private address: the ones plain
// http is allowed for.
func Local(ip netip.Addr) bool {
	ip = normalize(ip)
	return ip.IsLoopback() || ip.IsPrivate()
}

// CheckIP judges an address a connection is about to be made to. Unspecified,
// multicast, link-local (cloud metadata lives at 169.254.169.254) and
// carrier-grade NAT addresses are never allowed.
func CheckIP(ip netip.Addr) error {
	if !ip.IsValid() {
		return blocked("the address is not valid")
	}
	n := normalize(ip)
	switch {
	case n.IsUnspecified():
		return blocked("%s is the unspecified address", ip)
	case n.IsMulticast():
		return blocked("%s is a multicast address", ip)
	case n.IsLinkLocalUnicast():
		return blocked("%s is a link-local address (cloud metadata lives there)", ip)
	case cgnat.Contains(n):
		return blocked("%s is a carrier-grade NAT address", ip)
	case n == netip.AddrFrom4([4]byte{255, 255, 255, 255}):
		return blocked("%s is the broadcast address", ip)
	}
	return nil
}

// CheckURL judges a base URL with the default policy: no own address to
// refuse and the system resolver.
func CheckURL(raw string) (*url.URL, error) { return Policy{}.CheckURL(context.Background(), raw) }

// CheckURL judges a base URL. The scheme is http or https, there is a host
// and neither credentials nor a fragment. Plain http is allowed only for a
// loopback or private address, or for a name that resolves to nothing else:
// http://localhost:11434/v1 and a GPU box on the LAN work, a public host
// needs https.
func (p Policy) CheckURL(ctx context.Context, raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, blocked("the base URL is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		// url.Parse quotes its input, which may hold credentials.
		return nil, blocked("the base URL is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, blocked("the base URL must start with http:// or https://")
	}
	if u.User != nil {
		return nil, blocked("the base URL must not contain a user name or password")
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return nil, blocked("the base URL must not contain a fragment")
	}
	if u.RawQuery != "" || u.ForceQuery {
		return nil, blocked("the base URL must not contain a query")
	}
	host := u.Hostname()
	if host == "" {
		return nil, blocked("the base URL has no host")
	}
	if port := u.Port(); port != "" {
		if _, err := net.LookupPort("tcp", port); err != nil {
			return nil, blocked("the base URL has an invalid port")
		}
	}
	ips, err := p.lookup(ctx, host)
	if err != nil {
		if u.Scheme == "http" {
			return nil, &Error{Unresolved: true, Reason: fmt.Sprintf(
				"plain http is only allowed for local and private addresses, and %s could not be resolved", host)}
		}
		// https to a name that does not resolve right now is not a policy
		// matter: the connection will fail and say so.
		return u, nil
	}
	local := true
	for _, ip := range ips {
		if err := CheckIP(ip); err != nil {
			return nil, err
		}
		if p.self(ip, portOf(u)) {
			return nil, blocked("%s is this kubedojo server itself", u.Host)
		}
		local = local && Local(ip)
	}
	if u.Scheme == "http" && !local {
		return nil, blocked("plain http is only allowed for local and private addresses; use https for %s", host)
	}
	return u, nil
}

// lookup returns the addresses of a host: itself if it is an IP literal,
// loopback for localhost, otherwise what the resolver says.
func (p Policy) lookup(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip}, nil
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return []netip.Addr{netip.AddrFrom4([4]byte{127, 0, 0, 1})}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	found, err := p.resolver().LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var out []netip.Addr
	for _, a := range found {
		if ip, ok := netip.AddrFromSlice(a.IP); ok {
			out = append(out, ip.WithZone(a.Zone))
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no addresses")
	}
	return out, nil
}

func portOf(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "http" {
		return "80"
	}
	return "443"
}

// self reports whether ip:port is the address this server listens on.
func (p Policy) self(ip netip.Addr, port string) bool {
	if p.SelfAddr == "" {
		return false
	}
	host, selfPort, err := net.SplitHostPort(p.SelfAddr)
	if err != nil || selfPort != port {
		return false
	}
	ip = normalize(ip)
	own, err := netip.ParseAddr(host)
	if err != nil {
		// A listen address like ":4400" or "localhost:4400".
		return ip.IsLoopback() || host == "" && p.mine(ip)
	}
	own = normalize(own)
	switch {
	case own == ip:
		return true
	case own.IsUnspecified():
		// Listening on every interface: any address of this machine is us.
		return ip.IsLoopback() || p.mine(ip)
	case own.IsLoopback():
		return ip.IsLoopback()
	}
	return false
}

func (p Policy) mine(ip netip.Addr) bool {
	list := p.interfaces
	if list == nil {
		list = interfaceAddrs
	}
	for _, a := range list() {
		if normalize(a) == ip {
			return true
		}
	}
	return false
}

func interfaceAddrs() []netip.Addr {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []netip.Addr
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			if ip, ok := netip.AddrFromSlice(n.IP); ok {
				out = append(out, ip)
			}
		}
	}
	return out
}

// ClientOptions are the time limits of a guarded client.
type ClientOptions struct {
	// Connect bounds one connection attempt, TLS handshake included.
	// Zero means 5 seconds.
	Connect time.Duration
	// FirstByte bounds the wait for the response headers. Zero means no limit.
	FirstByte time.Duration
}

// Client returns the HTTP client for one checked base URL.
//
// Every connection it makes is judged on the IP address it dials, after the
// name was resolved. With an http base URL only loopback and private
// addresses are dialled. It follows no redirects: a redirect would lead to a
// host nobody checked. It uses no proxy from the environment for a local
// target; for a public https target the proxy of the environment applies,
// and then the address judged is the proxy's.
func (p Policy) Client(base *url.URL, o ClientOptions) *http.Client {
	if o.Connect <= 0 {
		o.Connect = 5 * time.Second
	}
	localOnly := base.Scheme == "http"
	check := func(ip netip.Addr, port string) error {
		if err := CheckIP(ip); err != nil {
			return err
		}
		if p.self(ip, port) {
			return blocked("%s is this kubedojo server itself", net.JoinHostPort(ip.String(), port))
		}
		if localOnly && !Local(ip) {
			return blocked("plain http is only allowed for local and private addresses, not %s", ip)
		}
		return nil
	}
	dialer := &net.Dialer{
		Timeout: o.Connect,
		// The last word: whatever resolved the name, this is the address the
		// socket connects to.
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return blocked("cannot judge the address %q", address)
			}
			return check(ap.Addr(), fmt.Sprint(ap.Port()))
		},
	}
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := p.lookup(ctx, host)
		if err != nil {
			return nil, &net.OpError{Op: "dial", Net: network, Err: &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}}
		}
		var last error
		for _, ip := range ips {
			// Refuse before dialling: a name with one bad address is not
			// tried on its good ones.
			if err := check(ip, port); err != nil {
				return nil, &net.OpError{Op: "dial", Net: network, Err: err}
			}
		}
		for _, ip := range ips {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}
	tr := &http.Transport{
		DialContext:           dial,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   o.Connect,
		ResponseHeaderTimeout: o.FirstByte,
		MaxIdleConns:          4,
		IdleConnTimeout:       60 * time.Second,
	}
	if !localOnly && !p.localTarget(base) {
		tr.Proxy = http.ProxyFromEnvironment
	}
	return &http.Client{
		Transport: tr,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// localTarget reports whether the host of base is, as far as can be told now,
// a loopback or private address.
func (p Policy) localTarget(base *url.URL) bool {
	ips, err := p.lookup(context.Background(), base.Hostname())
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if !Local(ip) {
			return false
		}
	}
	return true
}

// Origin is scheme://host:port of a base URL, with the default port spelled
// out and the host in lower case. Two base URLs with the same origin reach
// the same server.
func Origin(u *url.URL) string {
	if u == nil {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), portOf(u))
}

// OriginOf is Origin of a URL given as text; empty if it does not parse.
func OriginOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	return Origin(u)
}
