package session

import "crypto/rand"

// The characters of a session ID: lower case letters and digits without the
// ones that are mistaken for each other (i, l, o, 0, 1). An ID is part of
// container, network and Pod names (kd-<id>-terminal, kd-sandbox-<id>) and
// the value of a label, so it stays within what those allow.
const (
	idAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	idLength   = 12
)

// newID returns a new session ID. Whoever knows the ID of a session reaches
// its terminal, and with it a root shell in the sandbox: the ID comes from
// the system's random source and has close to 60 bits, too many to guess.
func newID() string {
	const n = len(idAlphabet)
	// A byte below limit maps to a character without favouring any; the few
	// above it are skipped.
	const limit = 256 - 256%n
	id := make([]byte, 0, idLength)
	buf := make([]byte, 2*idLength)
	for {
		_, _ = rand.Read(buf) // never fails: the process ends if there is no randomness
		for _, b := range buf {
			if int(b) >= limit {
				continue
			}
			if id = append(id, idAlphabet[int(b)%n]); len(id) == idLength {
				return string(id)
			}
		}
	}
}
