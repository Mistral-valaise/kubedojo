// Package catalog loads and validates training content: tracks (CKA, CKAD, CKS),
// their domains and lessons, and the questions with their automated checks.
package catalog

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"gopkg.in/yaml.v3"
)

type Catalog struct {
	Tracks map[string]*Track
	// Path is the recommended order through the tracks, nil if the content has none.
	Path  *Path
	order []string
}

// Track kinds. They decide how a track is presented, never how it is graded.
const (
	KindCertification = "certification" // practice for a hands-on certification exam (default)
	KindSkills        = "skills"        // hands-on skills without an exam behind them
	KindTheory        = "theory"        // practice for a multiple-choice exam; needs no sandbox
)

type Track struct {
	ID                string                      `yaml:"id" json:"id"`
	Title             string                      `yaml:"title" json:"title"`
	Short             string                      `yaml:"short" json:"short"`
	Description       string                      `yaml:"description" json:"description"`
	Kind              string                      `yaml:"kind" json:"kind"`
	Docs              []Link                      `yaml:"docs" json:"docs,omitempty"`
	KubernetesVersion string                      `yaml:"kubernetesVersion" json:"kubernetesVersion,omitempty"`
	Exam              ExamSpec                    `yaml:"exam" json:"exam"`
	Domains           []*Domain                   `yaml:"domains" json:"domains"`
	Environments      map[string]*sandbox.EnvSpec `yaml:"environments" json:"environments"`
	Questions         []*Question                 `yaml:"-" json:"-"`
	byID              map[string]*Question
}

// Sandboxless reports whether the track's questions are answered without a
// sandbox: no Docker, no cluster, nothing to provision.
func (t *Track) Sandboxless() bool { return t.Kind == KindTheory }

// NeedsCluster reports whether any environment of the track is a cluster.
func (t *Track) NeedsCluster() bool {
	for _, e := range t.Environments {
		if e.Type != sandbox.EnvContainer {
			return true
		}
	}
	return false
}

// NeedsContainer reports whether any environment of the track is a Linux
// container.
func (t *Track) NeedsContainer() bool {
	for _, e := range t.Environments {
		if e.Type == sandbox.EnvContainer {
			return true
		}
	}
	return false
}

// PlaceholderLesson is the sentence a lesson consists of while the real one
// is still being written.
const PlaceholderLesson = "This lesson is a placeholder"

// Draft reports whether the track is a skeleton: at least one of its lessons
// is still a placeholder. `content lint` says so, and the tests that hold a
// finished track to its size (questions per domain, enough for an exam) wait
// for the lessons before they apply.
func (t *Track) Draft() bool {
	for _, d := range t.Domains {
		if strings.Contains(d.Lesson, PlaceholderLesson) {
			return true
		}
	}
	return false
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
	ID    string `yaml:"id" json:"id"`
	Title string `yaml:"title" json:"title"`
	// Type is task (a directory with scripts and checks) or choice (one YAML
	// file with options). The shape on disk decides, not a field.
	Type        string   `yaml:"-" json:"type"`
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
	FreshEnvironment bool     `yaml:"freshEnvironment" json:"freshEnvironment"`
	Task             string   `yaml:"task" json:"-"`
	Hints            []string `yaml:"hints" json:"-"`
	Checks           []*Check `yaml:"checks" json:"-"`
	Setup            string   `yaml:"-" json:"-"`
	Cleanup          string   `yaml:"-" json:"-"`
	SolutionSh       string   `yaml:"-" json:"-"`
	SolutionMd       string   `yaml:"-" json:"-"`
	Track            string   `yaml:"-" json:"track"`
	Points           int      `yaml:"-" json:"points"`
	// Variants are alternative scripts under variants/*.sh that `content test`
	// runs instead of solution.sh: other valid solutions that must pass, and
	// plausible wrong ones that must fail specific checks.
	Variants []*Variant `yaml:"-" json:"-"`
	// Warnings are lint findings that do not make the content invalid.
	Warnings []string `yaml:"-" json:"-"`
	// Choice is set for multiple-choice questions and never serialised: it
	// holds the correct answer.
	Choice *Choice `yaml:"-" json:"-"`
}

// RunsOn reports whether the question can be offered by a server with the
// given sandbox driver. Choice questions need no sandbox at all.
func (q *Question) RunsOn(driver string) bool { return q.Type == TypeChoice || q.Driver == driver }

// Variant is a scripted answer with a declared grading outcome. The first
// lines of the script carry it: "# expect: pass" or "# expect: fail <check ids>".
type Variant struct {
	Name   string
	Script string
	Pass   bool
	Fails  []string // check ids that must fail; every other check must pass
}

// Check is one graded assertion. Every type boils down to "run something on a
// machine in the sandbox and compare its output", which keeps checks
// transparent: what the grader ran is what the candidate could run.
type Check struct {
	ID      string `yaml:"id" json:"id"`
	Title   string `yaml:"title" json:"title"`
	Points  int    `yaml:"points" json:"points"`
	Type    string `yaml:"type" json:"type"` // resource | file | exec
	On      string `yaml:"on" json:"on"`     // cluster: control-plane (default) | worker | worker2 | terminal; container: host (default) | terminal
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

	// Timeout bounds one run of the check (default 60s). Raise it for checks
	// that start a probe Pod or wait for a rollout.
	Timeout string `yaml:"timeout" json:"timeout,omitempty"`
	// Retry re-runs a failing check until it passes or this much time went by.
	// Use it for state that converges on its own (logs, endpoints, rollouts).
	Retry string `yaml:"retry" json:"retry,omitempty"`
	// Guard marks a check that protects against a destructive shortcut and
	// therefore legitimately passes on the untouched environment ("the API
	// server is healthy", "the base was not edited"). Every other check must
	// fail before the question is solved.
	Guard bool `yaml:"guard" json:"guard,omitempty"`

	TimeoutD time.Duration `yaml:"-" json:"-"`
	RetryD   time.Duration `yaml:"-" json:"-"`
}

const (
	DefaultCheckTimeout = 60 * time.Second
	MaxCheckTimeout     = 5 * time.Minute
	MaxCheckRetry       = 3 * time.Minute
)

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
	p, perrs := loadPath(fsys, c.Tracks)
	errs = append(errs, perrs...)
	c.Path = p
	if p != nil {
		// Tracks of the path come first, in its order; the rest stay alphabetical.
		var ordered []string
		on := map[string]bool{}
		for _, s := range p.Steps {
			if c.Tracks[s.Track] != nil && !on[s.Track] {
				ordered = append(ordered, s.Track)
				on[s.Track] = true
			}
		}
		for _, id := range c.order {
			if !on[id] {
				ordered = append(ordered, id)
			}
		}
		c.order = ordered
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
	switch t.Kind {
	case "":
		t.Kind = KindCertification
	case KindCertification, KindSkills, KindTheory:
	default:
		fail("kind must be %s, %s or %s", KindCertification, KindSkills, KindTheory)
	}
	if t.Kind == KindCertification && t.KubernetesVersion == "" {
		fail("kubernetesVersion is required for a certification track")
	}
	for _, d := range t.Docs {
		if d.Title == "" || !strings.HasPrefix(d.URL, "https://") {
			fail("docs: title and an https url are required")
		}
	}
	types := map[string]bool{}
	for name, e := range t.Environments {
		e.Name = name
		switch e.Type {
		case "":
			e.Type = sandbox.EnvCluster
		case sandbox.EnvCluster:
		case sandbox.EnvContainer:
			if e.Workers != 0 || e.KindConfig != "" {
				fail("environment %q: a container has no workers and no kindConfig", name)
			}
		default:
			fail("environment %q: type must be %s or %s", name, sandbox.EnvCluster, sandbox.EnvContainer)
		}
		types[e.Type] = true
	}
	if len(types) > 1 {
		// One session has one terminal on one network; mixing would need both.
		fail("environments mix clusters and containers; a track uses one type")
	}
	if t.Kind == KindTheory && len(t.Environments) > 0 {
		fail("a theory track has no environments: its questions need no sandbox")
	}
	if t.Environments == nil {
		// Serialised as {} rather than null: clients iterate over it.
		t.Environments = map[string]*sandbox.EnvSpec{}
	}

	entries, _ := fs.ReadDir(fsys, path.Join(dir, "questions"))
	for _, e := range entries {
		qp := path.Join(dir, "questions", e.Name())
		var q *Question
		var qerrs []error
		switch {
		case e.IsDir():
			if t.Kind == KindTheory {
				errs = append(errs, fmt.Errorf("%s: a theory track has choice questions only (one .yaml file each)", qp))
				continue
			}
			q, qerrs = loadQuestion(fsys, qp, e.Name(), t)
		case strings.HasSuffix(e.Name(), ".yaml"):
			if t.Kind != KindTheory {
				errs = append(errs, fmt.Errorf("%s: choice questions belong to a track with kind: %s", qp, KindTheory))
				continue
			}
			q, qerrs = loadChoice(fsys, qp, strings.TrimSuffix(e.Name(), ".yaml"), t)
		default:
			continue
		}
		for _, e := range qerrs {
			errs = append(errs, fmt.Errorf("%s: %w", qp, e))
		}
		if q != nil {
			if t.byID[q.ID] != nil {
				errs = append(errs, fmt.Errorf("%s: duplicate question id %q", qp, q.ID))
				continue
			}
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
	q := &Question{Type: TypeTask}
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
	env, ok := t.Environments[q.Environment]
	if !ok {
		fail("unknown environment %q", q.Environment)
		env = &sandbox.EnvSpec{Type: sandbox.EnvCluster}
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
		// An empty `on` means the environment's main machine. Spelling it out
		// here keeps the grader free of environment knowledge.
		if c.On == "" {
			c.On = env.DefaultRole()
		} else if !slices.Contains(env.Roles(), c.On) {
			fail("check %s: on must be one of %s in a %s environment, got %q", c.ID, strings.Join(env.Roles(), ", "), env.Type, c.On)
		}
		switch c.Type {
		case "resource":
			if c.Resource == "" {
				fail("check %s: resource is required", c.ID)
			}
			if env.Type == sandbox.EnvContainer {
				fail("check %s: a resource check needs a cluster; use file or exec in a container environment", c.ID)
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
		c.TimeoutD = DefaultCheckTimeout
		if c.Timeout != "" {
			d, err := time.ParseDuration(c.Timeout)
			if err != nil || d <= 0 || d > MaxCheckTimeout {
				fail("check %s: timeout must be a duration up to %s (e.g. 90s)", c.ID, MaxCheckTimeout)
			} else {
				c.TimeoutD = d
			}
		}
		if c.Retry != "" {
			d, err := time.ParseDuration(c.Retry)
			if err != nil || d <= 0 || d > MaxCheckRetry {
				fail("check %s: retry must be a duration up to %s (e.g. 30s)", c.ID, MaxCheckRetry)
			} else {
				c.RetryD = d
			}
		}
		for _, w := range fragile(c) {
			q.Warnings = append(q.Warnings, fmt.Sprintf("check %s: %s", c.ID, w))
		}
	}
	guards := 0
	for _, c := range q.Checks {
		if c.Guard {
			guards++
		}
	}
	if len(q.Checks) > 0 && guards == len(q.Checks) {
		fail("every check is a guard: nothing would fail on the untouched environment")
	}
	vs, verrs := loadVariants(fsys, path.Join(dir, "variants"), seen)
	q.Variants = vs
	errs = append(errs, verrs...)
	return q, errs
}

var (
	expectRe   = regexp.MustCompile(`(?m)^#\s*expect:\s*(pass|fail)\b(.*)$`)
	tailLogRe  = regexp.MustCompile(`\btail\s+(-n\s*)?-?\d+\s+/var/log/`)
	sleepRe    = regexp.MustCompile(`(^|[;&|\n]\s*)sleep\s+\d`)
	loopRe     = regexp.MustCompile(`\b(for|while|until)\b`)
	guardedRe  = regexp.MustCompile(`\bexit\s+[1-9]`)
	probePodRe = regexp.MustCompile(`\bkubectl\b[^\n|;&]*\srun\s[^\n|;&]*--rm\b`)
	cannotFail = regexp.MustCompile(`(\|\s*wc\s+-l(\s*\|\s*xargs)?|\|\|\s*true)\s*$`)
)

// fragile reports patterns in an exec check that have produced wrong grades:
// windows over log files, fixed sleeps, counters that read 0 when the
// command in front of them did not run at all, and throwaway probe Pods.
func fragile(c *Check) []string {
	if c.Type != "exec" {
		return nil
	}
	var out []string
	run := strings.TrimSpace(c.Run)
	if tailLogRe.MatchString(run) {
		out = append(out, "reads the tail of a log file; old lines decide the grade — compare against a baseline taken inside the check")
	}
	if sleepRe.MatchString(run) && c.RetryD == 0 && !loopRe.MatchString(run) {
		out = append(out, "uses a fixed sleep; poll for the expected state or set retry")
	}
	if c.Expect != nil && c.Expect.Equals != nil && strings.TrimSpace(*c.Expect.Equals) == "0" && cannotFail.MatchString(run) && !guardedRe.MatchString(run) {
		out = append(out, `expects "0" from a pipeline that also prints 0 when the command fails; assert that the object exists first`)
	}
	if probePodRe.MatchString(run) {
		out = append(out, "starts a throwaway Pod with kubectl run --rm; its output is lost when the Pod finishes before kubectl attaches — exec into a Pod the setup created, or probe from the node")
	}
	return out
}

func loadVariants(fsys fs.FS, dir string, checks map[string]bool) ([]*Variant, []error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, nil
	}
	var out []*Variant
	var errs []error
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sh") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".sh")
		raw := readOpt(fsys, path.Join(dir, e.Name()))
		m := expectRe.FindStringSubmatch(raw)
		if m == nil {
			errs = append(errs, fmt.Errorf("variants/%s: needs a header \"# expect: pass\" or \"# expect: fail <check ids>\"", e.Name()))
			continue
		}
		v := &Variant{Name: name, Script: raw, Pass: m[1] == "pass"}
		ids := strings.FieldsFunc(m[2], func(r rune) bool { return r == ' ' || r == ',' || r == '\t' })
		if v.Pass && len(ids) > 0 {
			errs = append(errs, fmt.Errorf("variants/%s: \"expect: pass\" takes no check ids", e.Name()))
		}
		if !v.Pass {
			if len(ids) == 0 {
				errs = append(errs, fmt.Errorf("variants/%s: \"expect: fail\" needs the ids of the checks that must fail", e.Name()))
			}
			for _, id := range ids {
				if !checks[id] {
					errs = append(errs, fmt.Errorf("variants/%s: unknown check id %q", e.Name(), id))
				}
			}
			v.Fails = ids
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, errs
}
