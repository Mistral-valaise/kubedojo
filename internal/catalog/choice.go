package catalog

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Question types. A task is solved in a sandbox and graded by checks; a choice
// question is answered by picking options and needs no sandbox.
const (
	TypeTask   = "task"
	TypeChoice = "choice"
)

// Choice is the answerable part of a multiple-choice question. It never
// leaves the server as part of a Question: which options are correct is
// revealed by the API only once the learner may see it.
type Choice struct {
	Multiple    bool
	Options     []*Option
	Explanation string
	References  []Link
}

type Option struct {
	// ID is derived from the text, so it survives reordering and says
	// nothing about the option's position.
	ID      string
	Text    string
	Correct bool
	Explain string
}

// Link is a titled URL: a reference of a question, or a track's documentation.
type Link struct {
	Title string `yaml:"title" json:"title"`
	URL   string `yaml:"url" json:"url"`
}

// Correct returns the ids of the correct options.
func (c *Choice) Correct() []string {
	var ids []string
	for _, o := range c.Options {
		if o.Correct {
			ids = append(ids, o.ID)
		}
	}
	return ids
}

// Grade reports whether answer is exactly the set of correct options. There
// is no partial credit: an exam question is right or wrong.
func (c *Choice) Grade(answer []string) bool {
	picked := map[string]bool{}
	for _, id := range answer {
		picked[id] = true
	}
	n := 0
	for _, o := range c.Options {
		if o.Correct != picked[o.ID] {
			return false
		}
		if o.Correct {
			n++
		}
	}
	return n == len(picked)
}

// choiceFile is questions/<id>.yaml.
type choiceFile struct {
	ID         string   `yaml:"id"`
	Title      string   `yaml:"title"`
	Domain     string   `yaml:"domain"`
	Difficulty string   `yaml:"difficulty"`
	Tags       []string `yaml:"tags"`
	Stem       string   `yaml:"stem"`
	Multiple   bool     `yaml:"multiple"`
	Options    []struct {
		Text    string `yaml:"text"`
		Correct bool   `yaml:"correct"`
		Explain string `yaml:"explain"`
	} `yaml:"options"`
	Explanation string `yaml:"explanation"`
	References  []Link `yaml:"references"`
}

var (
	spaceRe     = regexp.MustCompile(`\s+`)
	positionRe  = regexp.MustCompile(`(?i)\b(all|none|both|neither) of the (above|below|these|options)\b|\b(above|below)\b|\boption [a-f1-6]\b`)
	wordRe      = regexp.MustCompile(`[a-z0-9]+`)
	minOptions  = 3
	maxOptions  = 6
	wantOptions = 4
)

func normalise(s string) string {
	return strings.ToLower(spaceRe.ReplaceAllString(strings.TrimSpace(s), " "))
}

func optionID(text string) string {
	sum := sha1.Sum([]byte(normalise(text)))
	return hex.EncodeToString(sum[:])[:6]
}

// loadChoice reads a multiple-choice question from one YAML file.
func loadChoice(fsys fs.FS, file, name string, t *Track) (*Question, []error) {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	raw, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, []error{err}
	}
	var f choiceFile
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, []error{err}
	}
	q := &Question{ID: f.ID, Title: f.Title, Domain: f.Domain, Difficulty: f.Difficulty, Tags: f.Tags,
		Task: f.Stem, Track: t.ID, Type: TypeChoice, Points: 1, Minutes: 1}
	if q.ID == "" {
		q.ID = name
	}
	if !idRe.MatchString(q.ID) {
		fail("invalid id %q", q.ID)
	}
	if q.Title == "" || strings.TrimSpace(f.Stem) == "" {
		fail("title and stem are required")
	}
	if t.DomainByID(q.Domain) == nil {
		fail("unknown domain %q", q.Domain)
	}
	switch q.Difficulty {
	case "easy", "medium", "hard":
	case "":
		q.Difficulty = "medium"
	default:
		fail("difficulty must be easy, medium or hard")
	}

	c := &Choice{Multiple: f.Multiple, Explanation: strings.TrimSpace(f.Explanation), References: f.References}
	q.Choice = c
	if n := len(f.Options); n < minOptions || n > maxOptions {
		fail("%d options, want %d to %d", n, minOptions, maxOptions)
	} else if n != wantOptions {
		q.Warnings = append(q.Warnings, fmt.Sprintf("%d options; the exams use %d", n, wantOptions))
	}
	seen := map[string]bool{}
	correct, longestWrong, longestRight := 0, 0, 0
	for i, o := range f.Options {
		text := strings.TrimSpace(o.Text)
		if text == "" {
			fail("option %d has no text", i+1)
			continue
		}
		id := optionID(text)
		if seen[id] {
			fail("option %d repeats another option: %q", i+1, text)
		}
		seen[id] = true
		// Options are shuffled per session: one that refers to its position
		// or to the others stops making sense.
		if positionRe.MatchString(text) {
			q.Warnings = append(q.Warnings, fmt.Sprintf("option %q refers to other options or to its position; options are shown in random order", text))
		}
		if o.Correct {
			correct++
			longestRight = max(longestRight, len(text))
			if q.Title != "" && strings.Contains(normalise(q.Title), normalise(text)) && len(wordRe.FindAllString(normalise(text), -1)) > 0 {
				q.Warnings = append(q.Warnings, fmt.Sprintf("the title contains the correct answer %q; titles are shown in question lists", text))
			}
		} else {
			longestWrong = max(longestWrong, len(text))
		}
		c.Options = append(c.Options, &Option{ID: id, Text: text, Correct: o.Correct, Explain: strings.TrimSpace(o.Explain)})
	}
	switch {
	case len(c.Options) == 0:
	case !c.Multiple && correct != 1:
		fail("%d options are marked correct, want exactly 1 (set multiple: true for a select-all question)", correct)
	case c.Multiple && (correct < 2 || correct == len(c.Options)):
		fail("multiple: %d of %d options are correct, want at least 2 and not all", correct, len(c.Options))
	}
	// A correct answer that stands out by length can be guessed.
	if correct > 0 && longestWrong > 0 && longestRight*2 > longestWrong*3 && longestRight-longestWrong > 40 {
		q.Warnings = append(q.Warnings, "the correct option is much longer than every wrong one; the length gives it away")
	}
	if c.Explanation == "" {
		fail("explanation is required (say why the answer is right)")
	}
	if len(c.References) == 0 {
		fail("at least one reference is required (the documentation page that backs the answer)")
	}
	for _, r := range c.References {
		if r.Title == "" || !strings.HasPrefix(r.URL, "https://") {
			fail("reference %q: title and an https url are required", r.URL)
		}
	}
	return q, errs
}
