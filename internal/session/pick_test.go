package session

import (
	"testing"

	"github.com/kubedojo/kubedojo/content"
	"github.com/kubedojo/kubedojo/internal/catalog"
)

func TestPickQuestionsFollowsDomainWeights(t *testing.T) {
	cat, err := catalog.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range cat.List() {
		n := tr.Exam.Questions
		for round := 0; round < 20; round++ {
			picked := pickQuestions(tr, n, "kind")
			if len(picked) != n {
				t.Fatalf("%s: picked %d questions, want %d", tr.ID, len(picked), n)
			}
			seen := map[string]bool{}
			perDomain := map[string]int{}
			for _, q := range picked {
				if seen[q.ID] {
					t.Fatalf("%s: question %s picked twice", tr.ID, q.ID)
				}
				seen[q.ID] = true
				perDomain[q.Domain]++
			}
			for _, d := range tr.Domains {
				want := float64(n) * float64(d.Weight) / 100
				if got := float64(perDomain[d.ID]); got < want-1.01 || got > want+1.01 {
					t.Errorf("%s: domain %s got %v questions, want about %.1f", tr.ID, d.ID, got, want)
				}
			}
		}
	}
}

func TestPickQuestionsCapsAtAvailable(t *testing.T) {
	cat, _ := catalog.Load(content.FS)
	tr := cat.Tracks["cka"]
	if got := len(pickQuestions(tr, 1000, "kind")); got != len(tr.Questions) {
		t.Fatalf("picked %d, want all %d", got, len(tr.Questions))
	}
	if got := len(pickQuestions(tr, 5, "vm")); got != 0 {
		t.Fatalf("picked %d questions for a driver nothing uses", got)
	}
}
