package assistant

import (
	"regexp"
	"strings"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
)

// Diagnostics are read-only commands kubedojo runs in the sandbox for a
// failing check, so that the model sees the object or the file the check is
// about. The model never chooses them.
//
// A command is one of the fixed templates below. Its parameters come from
// the check's definition in the catalog and from nowhere else: not from the
// learner's message, not from the model, not from the output of a check.
// Each parameter has to match a strict pattern and is quoted for the shell
// all the same. A check that runs a free-form command gets no diagnostic,
// and a check's own command is never run again.

// Limits of the diagnostics of one message.
const (
	maxDiagnostics       = 3
	diagnosticOutputSize = 2 << 10
)

// diagnostic is one planned command.
type diagnostic struct {
	CheckID string
	// Role is the machine of the environment the check itself runs on.
	Role string
	// Display is the command as the model is told it.
	Display string
	// Script is what runs.
	Script string
}

var (
	// A resource kind, optionally qualified: deployment, hpa,
	// httproute.gateway.networking.k8s.io.
	kindRe = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)
	// An object name: a DNS subdomain.
	nameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)
	// A namespace: a DNS label.
	namespaceRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	// An absolute path of ordinary characters.
	pathRe = regexp.MustCompile(`^/[A-Za-z0-9._/@+-]{1,254}$`)
)

// shQuote quotes s for a POSIX shell. It is the same function as the
// checker's shQuote, which is not exported.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// secretKind reports whether a resource kind is a Secret, in any spelling
// kubectl accepts.
func secretKind(kind string) bool {
	base, _, _ := strings.Cut(kind, ".")
	return base == "secret" || base == "secrets"
}

// deniedPath reports whether a file may hold credentials. Such a file is not
// looked at, whatever the check says.
func deniedPath(p string) bool {
	l := strings.ToLower(p)
	base := l[strings.LastIndex(l, "/")+1:]
	switch {
	case l == "/etc/kubernetes/pki" || strings.HasPrefix(l, "/etc/kubernetes/pki/"),
		l == "/etc/shadow" || l == "/etc/gshadow" || strings.HasPrefix(l, "/etc/shadow") || strings.HasPrefix(l, "/etc/gshadow"),
		strings.HasPrefix(l, "/etc/kubernetes/") && strings.HasSuffix(l, ".conf"), // admin.conf and the other kubeconfigs
		strings.HasPrefix(l, "/proc/"), strings.HasPrefix(l, "/var/lib/kubelet/pki"), strings.HasPrefix(l, "/var/lib/etcd"),
		strings.HasSuffix(l, ".key"), strings.HasSuffix(l, ".pem"), strings.HasSuffix(l, ".p12"), strings.HasSuffix(l, ".pfx"),
		strings.Contains(l, "kubeconfig"), strings.Contains(l, "/.kube/"),
		strings.Contains(l, ".ssh"), strings.HasPrefix(base, "id_rsa"), strings.HasPrefix(base, "id_ed25519"), strings.HasPrefix(base, "id_ecdsa"),
		strings.Contains(l, "token"), strings.Contains(l, "secret"), strings.Contains(l, "password"), strings.Contains(l, "credential"):
		return true
	}
	return false
}

// diagnosticFor plans the diagnostic of one check, if its kind has one and
// its parameters are safe.
func diagnosticFor(c *catalog.Check) (diagnostic, bool) {
	d := diagnostic{CheckID: c.ID, Role: c.On}
	switch c.Type {
	case "resource":
		kind, name, ok := strings.Cut(c.Resource, "/")
		if !ok || !kindRe.MatchString(kind) || !nameRe.MatchString(name) || secretKind(kind) {
			return d, false
		}
		if c.Namespace != "" && !namespaceRe.MatchString(c.Namespace) {
			return d, false
		}
		object := shQuote(kind) + " " + shQuote(name)
		shown := kind + " " + name
		if c.Namespace != "" {
			object += " -n " + shQuote(c.Namespace)
			shown += " -n " + c.Namespace
		}
		d.Display = "kubectl get " + shown + " -o wide; kubectl describe " + shown + " | tail -n 25"
		d.Script = "kubectl get " + object + " -o wide 2>&1 | head -n 20\necho\nkubectl describe " + object + " 2>&1 | tail -n 25\n"
		return d, true
	case "file":
		p := c.Path
		if !pathRe.MatchString(p) || strings.Contains(p, "..") || strings.Contains(p, "//") || deniedPath(p) {
			return d, false
		}
		q := shQuote(p)
		d.Display = "ls -la -- " + p + "; head -n 40 -- " + p
		d.Script = "ls -la -- " + q + " 2>&1 | head -n 20\nif [ -f " + q + " ]; then echo; head -c 8192 -- " + q + " 2>&1 | head -n 40; fi\n"
		return d, true
	}
	// An exec check runs a command its author wrote. There is no template
	// for that, and the command itself is not run a second time.
	return d, false
}
