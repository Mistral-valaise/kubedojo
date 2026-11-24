package main

import (
	"flag"
	"net/http"
	"strings"

	"github.com/Mistral-valaise/kubedojo/internal/httpsec"
)

// secFlags are the flags that protect the HTTP server. `serve` and `up` have
// them.
type secFlags struct {
	allowedHosts *string
}

// registerSecFlags registers the security flags on fl.
func registerSecFlags(fl *flag.FlagSet) *secFlags {
	return &secFlags{
		allowedHosts: fl.String("allowed-hosts", env("KUBEDOJO_ALLOWED_HOSTS", ""),
			"more Host names the server answers to, comma-separated (localhost and the listen address always are): "+
				"kubedojo.example.com, *.example.com for subdomains, * to accept any Host"),
	}
}

// hosts returns the entries of --allowed-hosts.
func (f *secFlags) hosts() []string {
	if f == nil || f.allowedHosts == nil {
		return nil
	}
	var out []string
	for _, h := range strings.Split(*f.allowedHosts, ",") {
		if h = strings.TrimSpace(h); h != "" {
			out = append(out, h)
		}
	}
	return out
}

// wrap puts the checks the flags configure in front of the handler. It is the
// outermost handler of the server: a request it rejects reaches nothing else,
// neither the handlers nor the request span and the HTTP metrics around them.
// p says how the server runs (its address, the dev origin, hosted or not).
//
// The order is: recover from a panic, check the Host, reject state-changing
// requests of other origins, set the security headers, then h.
func (f *secFlags) wrap(h http.Handler, p serverParts) http.Handler {
	return httpsec.Chain(httpsec.Config{ListenAddr: p.addr, AllowedHosts: f.hosts(), DevOrigin: p.devOrigin, Log: p.log})(h)
}
