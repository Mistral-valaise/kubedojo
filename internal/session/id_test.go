package session

import (
	"regexp"
	"strings"
	"testing"
)

func TestNewID(t *testing.T) {
	// What the names built from an ID must satisfy: a Docker container and
	// network name, a Pod name (a DNS label of at most 63 characters) and a
	// label value.
	dnsLabel := regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	seen := map[string]bool{}
	counts := map[rune]int{}
	const n = 10000
	for range n {
		id := newID()
		if len(id) != 12 {
			t.Fatalf("newID() = %q: %d characters, want 12", id, len(id))
		}
		for _, c := range id {
			if !strings.ContainsRune(idAlphabet, c) {
				t.Fatalf("newID() = %q: %q is not in the alphabet", id, c)
			}
			counts[c]++
		}
		if seen[id] {
			t.Fatalf("newID() returned %q twice in %d IDs", id, n)
		}
		seen[id] = true
		for _, name := range []string{"kd-sandbox-" + id, "kd-" + id + "-terminal", "kd-" + id, id} {
			if !dnsLabel.MatchString(name) {
				t.Fatalf("%q is not a DNS label", name)
			}
		}
	}
	// Every character is used, and none far more often than another: 120000
	// characters over 31 are about 3871 each.
	for _, c := range idAlphabet {
		if counts[c] < 3400 || counts[c] > 4350 {
			t.Errorf("%q appears %d times in %d IDs, want about %d", c, counts[c], n, n*12/len(idAlphabet))
		}
	}
	if strings.ContainsAny(idAlphabet, "ilo01") || len(idAlphabet) != 31 {
		t.Errorf("the alphabet changed: %q", idAlphabet)
	}
}
