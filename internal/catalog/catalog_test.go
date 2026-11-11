package catalog_test

import (
	"testing"

	"github.com/Mistral-valaise/kubedojo/content"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
)

// The shipped content must always be valid.
func TestShippedContentIsValid(t *testing.T) {
	cat, err := catalog.Load(content.FS)
	if err != nil {
		t.Fatalf("content is invalid:\n%v", err)
	}
	for _, id := range []string{"cka", "cks"} {
		tr := cat.Tracks[id]
		if tr == nil {
			t.Fatalf("track %s is missing", id)
		}
		perDomain := map[string]int{}
		for _, q := range tr.Questions {
			perDomain[q.Domain]++
			if len(q.Hints) == 0 {
				t.Errorf("%s/%s has no hints", id, q.ID)
			}
		}
		for _, d := range tr.Domains {
			if perDomain[d.ID] == 0 {
				t.Errorf("%s: domain %s has no questions", id, d.ID)
			}
		}
		if len(tr.Questions) < tr.Exam.Questions {
			t.Errorf("%s has %d questions, fewer than one exam (%d)", id, len(tr.Questions), tr.Exam.Questions)
		}
	}
}
