package api

import (
	"strings"
	"testing"

	"github.com/Mistral-valaise/kubedojo/content"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
)

// The tutor's snapshot runs on the main machine of a question's environment.
// On a Linux host there is no kubectl, and a snapshot full of "command not
// found" would send the tutor looking for a cluster problem.
func TestTutorSnapshotFitsTheEnvironment(t *testing.T) {
	cat, err := catalog.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Catalog: cat}
	seen := map[bool]bool{}
	for _, tr := range cat.List() {
		for _, q := range tr.Questions {
			if q.Type != catalog.TypeTask {
				continue
			}
			host := tr.NeedsContainer()
			seen[host] = true
			if got := strings.Contains(s.tutorSnapshot(q), "kubectl"); got == host {
				t.Fatalf("%s/%s: snapshot uses kubectl = %v on a host = %v", tr.ID, q.ID, got, host)
			}
		}
	}
	if !seen[true] || !seen[false] {
		t.Errorf("the shipped content no longer has both environment types: %v", seen)
	}
}
