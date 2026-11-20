package api_test

import (
	"net/http"
	"testing"
)

// Readiness follows the shutdown; liveness does not, or Kubernetes would kill
// a server that is still finishing an exam.
func TestReadyFollowsDrain(t *testing.T) {
	f := newFixture(t, 1)
	status := func(path string) int {
		res, err := http.Get(f.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if got := status("/api/ready"); got != 200 {
		t.Fatalf("/api/ready = %d, want 200", got)
	}
	f.api.Drain()
	if got := status("/api/ready"); got != 503 {
		t.Fatalf("/api/ready while draining = %d, want 503", got)
	}
	if got := status("/api/health"); got != 200 {
		t.Fatalf("/api/health while draining = %d, want 200", got)
	}
}
