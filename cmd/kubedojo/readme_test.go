package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
)

// The README states how many questions there are, in total and per track.
// Those numbers went stale more than once; they must follow the content.
func TestReadmeQuestionCounts(t *testing.T) {
	cat, err := catalog.Load(os.DirFS("../../content"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(raw)

	total := 0
	for _, tr := range cat.Tracks {
		total += len(tr.Questions)
	}
	for name, re := range map[string]*regexp.Regexp{
		"badge":         regexp.MustCompile(`badge/questions-(\d+)_verified`),
		"feature table": regexp.MustCompile(`(\d+) original questions`),
	} {
		m := re.FindStringSubmatch(readme)
		if m == nil {
			t.Errorf("%s: the README no longer states the number of questions", name)
			continue
		}
		if got, _ := strconv.Atoi(m[1]); got != total {
			t.Errorf("%s: the README says %d questions, the content has %d", name, got, total)
		}
	}

	// One row per track: | 5 | **CKS** | hands-on, clusters | 29 tasks: … |
	row := regexp.MustCompile(`(?m)^\|[^|]*\| \*\*([^*]+)\*\* \|[^|]*\| (\d+) `)
	seen := map[string]bool{}
	for _, m := range row.FindAllStringSubmatch(readme, -1) {
		// "Linux & Vim Essentials" is the track linux.
		id, _, _ := strings.Cut(strings.ToLower(m[1]), " ")
		tr := cat.Tracks[id]
		if tr == nil {
			continue
		}
		seen[id] = true
		if got, _ := strconv.Atoi(m[2]); got != len(tr.Questions) {
			t.Errorf("track %s: the README says %d questions, the content has %d", id, got, len(tr.Questions))
		}
	}
	for id := range cat.Tracks {
		if !seen[id] {
			t.Errorf("track %s: no row with a question count in the README", id)
		}
	}
}
