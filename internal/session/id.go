package session

import "math/rand/v2"

func newID() string {
	const abc = "abcdefghjkmnpqrstuvwxyz23456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = abc[rand.IntN(len(abc))]
	}
	return string(b)
}
