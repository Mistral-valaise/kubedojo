package api

import (
	"fmt"
	"strings"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/session"
)

// choiceView is what a learner sees of a multiple-choice question. The
// answer key (review) is added only once the session reveals the question;
// everything else here is safe to show at any time.
type choiceView struct {
	Multiple bool         `json:"multiple"`
	Options  []optionView `json:"options"` // in the order drawn for this session
	Answer   []string     `json:"answer"`  // the learner's own picks
	Review   *reviewView  `json:"review,omitempty"`
}

type optionView struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type reviewView struct {
	Correct     []string          `json:"correct"`
	Explanation string            `json:"explanation"`
	Options     map[string]string `json:"options"` // option id -> why it is right or wrong
	References  []catalog.Link    `json:"references"`
}

func review(c *catalog.Choice) *reviewView {
	r := &reviewView{Correct: c.Correct(), Explanation: c.Explanation, Options: map[string]string{}, References: c.References}
	for _, o := range c.Options {
		if o.Explain != "" {
			r.Options[o.ID] = o.Explain
		}
	}
	return r
}

func viewChoice(v *session.Session, q *catalog.Question, qs *session.QState) *choiceView {
	c := q.Choice
	out := &choiceView{Multiple: c.Multiple, Options: []optionView{}, Answer: qs.Answer}
	if out.Answer == nil {
		out.Answer = []string{}
	}
	text := map[string]string{}
	for _, o := range c.Options {
		text[o.ID] = o.Text
	}
	// An id that is no longer in the catalog (the option was reworded after
	// the session was created) is dropped rather than shown without text.
	for _, id := range qs.Order {
		if t, ok := text[id]; ok {
			out.Options = append(out.Options, optionView{ID: id, Text: t})
		}
	}
	if v.Revealed(qs) {
		out.Review = review(c)
	}
	return out
}

// solutionMarkdown renders the review as the worked solution of a choice
// question, for clients that show every solution as markdown.
func solutionMarkdown(c *catalog.Choice) string {
	var b strings.Builder
	b.WriteString("**Correct answer**\n\n")
	for _, o := range c.Options {
		if o.Correct {
			fmt.Fprintf(&b, "- %s\n", o.Text)
		}
	}
	fmt.Fprintf(&b, "\n%s\n", c.Explanation)
	if len(c.References) > 0 {
		b.WriteString("\n**References**\n\n")
		for _, r := range c.References {
			fmt.Fprintf(&b, "- [%s](%s)\n", r.Title, r.URL)
		}
	}
	return b.String()
}
