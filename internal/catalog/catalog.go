// Package catalog loads and validates training content: tracks (CKA, CKS),
// their domains and lessons, and the questions with their automated checks.
package catalog

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/kubedojo/kubedojo/internal/sandbox"
	"gopkg.in/yaml.v3"
)

type Catalog struct {
	Tracks map[string]*Track
	order  []string
}

type Track struct {
	ID                string                      `yaml:"id" json:"id"`
	Title             string                      `yaml:"title" json:"title"`
	Short             string                      `yaml:"short" json:"short"`
	Description       string                      `yaml:"description" json:"description"`
	KubernetesVersion string                      `yaml:"kubernetesVersion" json:"kubernetesVersion"`
	Exam              ExamSpec                    `yaml:"exam" json:"exam"`
	Domains           []*Domain                   `yaml:"domains" json:"domains"`
	Environments      map[string]*sandbox.EnvSpec `yaml:"environments" json:"environments"`
	Questions         []*Question                 `yaml:"-" json:"-"`
	byID              map[string]*Question
}

type ExamSpec struct {
	DurationMinutes int `yaml:"durationMinutes" json:"durationMinutes"`
	PassPercent     int `yaml:"passPercent" json:"passPercent"`
	Questions       int `yaml:"questions" json:"questions"`
}

type Domain struct {
	ID      string `yaml:"id" json:"id"`
	Title   string `yaml:"title" json:"title"`
	Weight  int    `yaml:"weight" json:"weight"`
	Summary string `yaml:"summary" json:"summary"`
	Lesson  string `yaml:"-" json:"-"` // markdown
}

type Question struct {
	ID          string   `yaml:"id" json:"id"`
	Title       string   `yaml:"title" json:"title"`
	Domain      string   `yaml:"domain" json:"domain"`
	Difficulty  string   `yaml:"difficulty" json:"difficulty"`
	Minutes     int      `yaml:"minutes" json:"minutes"`
	Environment string   `yaml:"environment" json:"environment"`
	Driver      string   `yaml:"driver" json:"driver"`
	Tags        []string `yaml:"tags" json:"tags"`
	// Disruptive questions break cluster-wide components (kubelet, scheduler,
	// DNS). They need a cleanup.sh that restores a healthy cluster, which
	// practice mode runs before another question is prepared.
	Disruptive bool `yaml:"disruptive" json:"disruptive"`
	// FreshEnvironment marks questions whose effects cannot be undone by a
	// script (e.g. encrypting etcd). Resetting them recreates the cluster.
	FreshEnvironment bool `yaml:"freshEnvironment" json:"freshEnvironment"`
	Task        string   `yaml:"task" json:"-"`
	Hints       []string `yaml:"hints" json:"-"`
	Checks      []*Check `yaml:"checks" json:"-"`
	Setup       string   `yaml:"-" json:"-"`
	Cleanup     string   `yaml:"-" json:"-"`
	SolutionSh  string   `yaml:"-" json:"-"`
	SolutionMd  string   `yaml:"-" json:"-"`
	Track       string   `yaml:"-" json:"track"`
	Points      int      `yaml:"-" json:"points"`
}

// Check is one graded assertion. Every type boils down to "run something on a
// machine in the sandbox and compare its output", which keeps checks
// transparent: what the grader ran is what the candidate could run.
type Check struct {
	ID      string `yaml:"id" json:"id"`
	Title   string `yaml:"title" json:"title"`
	Points  int    `yaml:"points" json:"points"`
	Type    string `yaml:"type" json:"type"` // resource | file | exec
	On      string `yaml:"on" json:"on"`     // control-plane (default) | worker | worker2 | terminal
	Explain string `yaml:"explain" json:"explain"`

	// resource
	Resource  string `yaml:"resource" json:"resource,omitempty"` // kind/name
	Namespace string `yaml:"namespace" json:"namespace,omitempty"`
	JSONPath  string `yaml:"jsonpath" json:"jsonpath,omitempty"`
	Absent    bool   `yaml:"absent" json:"absent,omitempty"`
	// file
	Path string `yaml:"path" json:"path,omitempty"`
	// exec
	Run string `yaml:"run" json:"run,omitempty"`

	Expect *Expect `yaml:"expect" json:"expect,omitempty"`
}

// Expect is a set of matchers; all given matchers must hold.
type Expect struct {
	Equals      *string  `yaml:"equals" json:"equals,omitempty"`
	NotEquals   *string  `yaml:"notEquals" json:"notEquals,omitempty"`
	Contains    []string `yaml:"contains" json:"contains,omitempty"`
	NotContains []string `yaml:"notContains" json:"notContains,omitempty"`
	Matches     string   `yaml:"matches" json:"matches,omitempty"`
	OneOf       []string `yaml:"oneOf" json:"oneOf,omitempty"`
	SameLines   []string `yaml:"sameLines" json:"sameLines,omitempty"` // same set of lines, any order
	SameItems   []string `yaml:"sameItems" json:"sameItems,omitempty"` // same set of whitespace separated items
	Gte         *float64 `yaml:"gte" json:"gte,omitempty"`
	Lte         *float64 `yaml:"lte" json:"lte,omitempty"`
	NotEmpty    bool     `yaml:"notEmpty" json:"notEmpty,omitempty"`
	ExitCode    *int     `yaml:"exitCode" json:"exitCode,omitempty"`
}

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Load reads every track under root of fsys.
func Load(fsys fs.FS) (*Catalog, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	c := &Catalog{Tracks: map[string]*Track{}}
	var errs []error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := fs.Stat(fsys, path.Join(e.Name(), "track.yaml")); err != nil {
			continue
		}
		t, terrs := loadTrack(fsys, e.Name())
		errs = append(errs, terrs...)
		if t != nil {
			c.Tracks[t.ID] = t
			c.order = append(c.order, t.ID)
		}
	}
	sort.Strings(c.order)
	if len(c.Tracks) == 0 && len(errs) == 0 {
		errs = append(errs, errors.New("no tracks found"))
	}
	return c, errors.Join(errs...)
}

func (c *Catalog) List() []*Track {
	out := make([]*Track, 0, len(c.order))
	for _, id := range c.order {
		out = append(out, c.Tracks[id])
	}
	return out
}

func (t *Track) Question(id string) *Question { return t.byID[id] }

func (t *Track) DomainByID(id string) *Domain {
	for _, d := range t.Domains {
		if d.ID == id {
			return d
		}
	}
	return nil
}

func readOpt(fsys fs.FS, p string) string {
	b, err := fs.ReadFile(fsys, p)
	if err != nil {
		return ""
	}
	return string(b)
}

func loadTrack(fsys fs.FS, dir string) (*Track, []error) {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(dir+": "+format, a...)) }

	raw, err := fs.ReadFile(fsys, path.Join(dir, "track.yaml"))
	if err != nil {
		return nil, []error{err}
	}
	t := &Track{byID: map[string]*Question{}}
	if err := yaml.Unmarshal(raw, t); err != nil {
		return nil, []error{fmt.Errorf("%s/track.yaml: %w", dir, err)}
	}
	if t.ID == "" {
		t.ID = dir
	}
	if t.Exam.DurationMinutes == 0 || t.Exam.PassPercent == 0 || t.Exam.Questions == 0 {
		fail("exam.durationMinutes, exam.passPercent and exam.questions are required")
	}
	total := 0
	for _, d := range t.Domains {
		total += d.Weight
		d.Lesson = readOpt(fsys, path.Join(dir, "lessons", d.ID+".md"))
		if d.Lesson == "" {
			fail("domain %q has no lesson (lessons/%s.md)", d.ID, d.ID)
		}
	}
	if total != 100 {
		fail("domain weights sum to %d, want 100", total)
	}
	for name, e := range t.Environments {
		e.Name = name
	}

	qdirs, _ := fs.ReadDir(fsys, path.Join(dir, "questions"))
	for _, qd := range qdirs {
		if !qd.IsDir() {
			continue
		}
		qp := path.Join(dir, "questions", qd.Name())
		q, qerrs := loadQuestion(fsys, qp, qd.Name(), t)
		for _, e := range qerrs {
			errs = append(errs, fmt.Errorf("%s: %w", qp, e))
		}
		if q != nil {
			t.Questions = append(t.Questions, q)
			t.byID[q.ID] = q
		}
	}
	sort.Slice(t.Questions, func(i, j int) bool { return t.Questions[i].ID < t.Questions[j].ID })
	return t, errs
}

func loadQuestion(fsys fs.FS, dir, name string, t *Track) (*Question, []error) {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	raw, err := fs.ReadFile(fsys, path.Join(dir, "question.yaml"))
	if err != nil {
		return nil, []error{err}
	}
	q := &Question{}
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(q); err != nil {
		return nil, []error{err}
	}
	if q.ID == "" {
		q.ID = name
	}
	q.Track = t.ID
	if !idRe.MatchString(q.ID) {
		fail("invalid id %q", q.ID)
	}
	if q.Title == "" || strings.TrimSpace(q.Task) == "" {
		fail("title and task are required")
	}
	if t.DomainByID(q.Domain) == nil {
		fail("unknown domain %q", q.Domain)
	}
	if q.Environment == "" {
		q.Environment = "base"
	}
	if _, ok := t.Environments[q.Environment]; !ok {
		fail("unknown environment %q", q.Environment)
	}
	if q.Driver == "" {
		q.Driver = "kind"
	}
	switch q.Difficulty {
	case "easy", "medium", "hard":
	case "":
		q.Difficulty = "medium"
	default:
		fail("difficulty must be easy, medium or hard")
	}
	if q.Minutes == 0 {
		q.Minutes = 7
	}
	q.Setup = readOpt(fsys, path.Join(dir, "setup.sh"))
	q.Cleanup = readOpt(fsys, path.Join(dir, "cleanup.sh"))
	q.SolutionSh = readOpt(fsys, path.Join(dir, "solution.sh"))
	q.SolutionMd = readOpt(fsys, path.Join(dir, "solution.md"))
	if q.SolutionSh == "" {
		fail("solution.sh is required (it proves the question is solvable)")
	}
	if q.SolutionMd == "" {
		fail("solution.md is required")
	}
	if q.Disruptive && q.Cleanup == "" {
		fail("disruptive questions need a cleanup.sh that restores a healthy cluster")
	}
	if len(q.Checks) == 0 {
		fail("at least one check is required")
	}
	seen := map[string]bool{}
	for i, c := range q.Checks {
		if c.ID == "" {
			c.ID = fmt.Sprintf("check-%d", i+1)
		}
		if seen[c.ID] {
			fail("duplicate check id %q", c.ID)
		}
		seen[c.ID] = true
		if c.Points == 0 {
			c.Points = 1
		}
		q.Points += c.Points
		if c.Title == "" {
			fail("check %s: title is required", c.ID)
		}
		if c.Explain == "" {
			fail("check %s: explain is required (tell the learner why this matters)", c.ID)
		}
		switch c.Type {
		case "resource":
			if c.Resource == "" {
				fail("check %s: resource is required", c.ID)
			}
		case "file":
			if c.Path == "" {
				fail("check %s: path is required", c.ID)
			}
		case "exec":
			if c.Run == "" {
				fail("check %s: run is required", c.ID)
			}
		default:
			fail("check %s: unknown type %q", c.ID, c.Type)
		}
		if c.Expect != nil && c.Expect.Matches != "" {
			if _, err := regexp.Compile(c.Expect.Matches); err != nil {
				fail("check %s: bad regex: %v", c.ID, err)
			}
		}
	}
	return q, errs
}
