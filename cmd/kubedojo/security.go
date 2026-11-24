package main

import (
	"flag"
	"net/http"
)

// secFlags are the flags that protect the HTTP server. `serve` and `up` have
// them. There are none yet: the security work adds --allowed-hosts here.
type secFlags struct{}

// registerSecFlags registers the security flags on fl.
func registerSecFlags(fl *flag.FlagSet) *secFlags {
	return &secFlags{}
}

// wrap puts the checks the flags configure in front of the handler. It is the
// outermost handler of the server: a request it rejects reaches nothing else.
// p says how the server runs (its address, the dev origin, hosted or not).
func (f *secFlags) wrap(h http.Handler, p serverParts) http.Handler { return h }
