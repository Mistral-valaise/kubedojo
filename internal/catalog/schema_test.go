package catalog_test

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
)

func file(body string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(body)} }

// loadErr loads fsys and returns the error text, "" when the content is valid.
func loadErr(fsys fstest.MapFS) string {
	if _, err := catalog.Load(fsys); err != nil {
		return err.Error()
	}
	return ""
}

func wantErr(t *testing.T, name, got, want string) {
	t.Helper()
	switch {
	case want == "" && got != "":
		t.Errorf("%s: unexpected error: %s", name, got)
	case want != "" && !strings.Contains(got, want):
		t.Errorf("%s: want an error containing %q, got: %q", name, want, got)
	}
}

const execQuestion = `
title: Q
domain: d
task: do
hints: [h]
checks:
  - {id: a, title: A, type: %s, %s explain: e}
`

func taskTrack(track, check string) fstest.MapFS {
	return fstest.MapFS{
		"t/track.yaml":                file(track),
		"t/lessons/d.md":              file("# D"),
		"t/questions/q/question.yaml": file(check),
		"t/questions/q/solution.sh":   file("true"),
		"t/questions/q/solution.md":   file("do it"),
	}
}

const head = "title: T\nexam: {durationMinutes: 10, passPercent: 50, questions: 1}\ndomains: [{id: d, title: D, weight: 100}]\n"

func TestTrackKindAndEnvironments(t *testing.T) {
	q := strings.NewReplacer("%s, %s", "exec, run: \"true\",").Replace(execQuestion)
	for name, c := range map[string]struct{ track, want string }{
		"certification is the default and needs a version": {head + "environments: {base: {}}\n", "kubernetesVersion is required"},
		"certification with a version":                     {head + "kubernetesVersion: v1.37\nenvironments: {base: {}}\n", ""},
		"skills needs no version":                          {head + "kind: skills\nenvironments: {base: {type: container}}\n", ""},
		"unknown kind":                                     {head + "kind: quiz\nenvironments: {base: {}}\n", "kind must be"},
		"unknown environment type":                         {head + "kind: skills\nenvironments: {base: {type: vm}}\n", "type must be"},
		"a container has no workers":                       {head + "kind: skills\nenvironments: {base: {type: container, workers: 1}}\n", "no workers"},
		"one environment type per track":                   {head + "kind: skills\nenvironments: {base: {type: container}, k: {}}\n", "mix clusters and containers"},
		"docs need https":                                  {head + "kind: skills\ndocs: [{title: Man, url: \"http://x\"}]\nenvironments: {base: {type: container}}\n", "https url"},
	} {
		wantErr(t, name, loadErr(taskTrack(c.track, q)), c.want)
	}
}

func TestCheckTargetIsValidatedPerEnvironment(t *testing.T) {
	cluster := head + "kubernetesVersion: v1.37\nenvironments: {base: {}}\n"
	container := head + "kind: skills\nenvironments: {base: {type: container}}\n"
	check := func(typ, rest string) string {
		return strings.Replace(strings.Replace(execQuestion, "%s", typ, 1), "%s", rest, 1)
	}
	for name, c := range map[string]struct{ track, question, want string }{
		"cluster: worker":              {cluster, check("exec", `run: "true", on: worker,`), ""},
		"cluster: a typo is caught":    {cluster, check("exec", `run: "true", on: wroker,`), "on must be one of"},
		"cluster: host is a container": {cluster, check("exec", `run: "true", on: host,`), "on must be one of"},
		"container: host":              {container, check("exec", `run: "true", on: host,`), ""},
		"container: terminal":          {container, check("file", `path: /tmp/x, on: terminal,`), ""},
		"container: no worker":         {container, check("exec", `run: "true", on: worker,`), "on must be one of"},
		"container: no kubectl":        {container, check("resource", `resource: pod/x,`), "needs a cluster"},
	} {
		wantErr(t, name, loadErr(taskTrack(c.track, c.question)), c.want)
	}

	// An omitted `on` is filled in with the environment's main machine.
	for track, want := range map[string]string{cluster: "control-plane", container: "host"} {
		cat, err := catalog.Load(taskTrack(track, check("exec", `run: "true",`)))
		if err != nil {
			t.Fatal(err)
		}
		if got := cat.Tracks["t"].Question("q").Checks[0].On; got != want {
			t.Errorf("default on = %q, want %q", got, want)
		}
	}
}

const theoryTrack = head + "kind: theory\n"

const choiceQuestion = `
title: Smallest deployable unit
domain: d
stem: Which object is the smallest deployable unit in Kubernetes?
options:
  - {text: Container, explain: A container always runs inside a Pod.}
  - {text: Pod, correct: true}
  - {text: Deployment}
  - {text: Node}
explanation: A Pod wraps one or more containers and is what the scheduler places.
references: [{title: Pods, url: "https://kubernetes.io/docs/concepts/workloads/pods/"}]
`

func theoryFS(question string) fstest.MapFS {
	return fstest.MapFS{
		"t/track.yaml":          file(theoryTrack),
		"t/lessons/d.md":        file("# D"),
		"t/questions/unit.yaml": file(question),
	}
}

func TestChoiceQuestion(t *testing.T) {
	cat, err := catalog.Load(theoryFS(choiceQuestion))
	if err != nil {
		t.Fatal(err)
	}
	tr := cat.Tracks["t"]
	if !tr.Sandboxless() || tr.NeedsCluster() {
		t.Error("a theory track needs neither sandbox nor cluster")
	}
	q := tr.Question("unit")
	if q == nil || q.Type != catalog.TypeChoice || q.Points != 1 || q.Choice == nil {
		t.Fatalf("question = %+v", q)
	}
	if !q.RunsOn("kind") || !q.RunsOn("anything") {
		t.Error("a choice question runs with every driver")
	}
	if len(q.Warnings) > 0 {
		t.Errorf("unexpected warnings: %v", q.Warnings)
	}
	correct := q.Choice.Correct()
	if len(correct) != 1 {
		t.Fatalf("correct = %v", correct)
	}
	var wrong string
	ids := map[string]bool{}
	for _, o := range q.Choice.Options {
		if len(o.ID) != 6 || ids[o.ID] {
			t.Errorf("option id %q is not a unique 6 character id", o.ID)
		}
		ids[o.ID] = true
		if !o.Correct {
			wrong = o.ID
		}
	}
	for name, c := range map[string]struct {
		answer []string
		want   bool
	}{
		"the correct option":      {correct, true},
		"nothing":                 {nil, false},
		"a wrong option":          {[]string{wrong}, false},
		"correct plus a wrong":    {[]string{correct[0], wrong}, false},
		"an id that is no option": {[]string{"zzzzzz"}, false},
		"correct plus an unknown": {[]string{correct[0], "zzzzzz"}, false},
		"the correct one twice":   {[]string{correct[0], correct[0]}, true},
	} {
		if got := q.Choice.Grade(c.answer); got != c.want {
			t.Errorf("Grade(%s) = %v, want %v", name, got, c.want)
		}
	}

	// What a client can be sent about a question must not give the answer away.
	b, _ := json.Marshal(q)
	for _, secret := range []string{correct[0], "A Pod wraps", "always runs inside", `"Choice"`, "options", "explanation"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("the question's JSON leaks %q: %s", secret, b)
		}
	}
}

func TestChoiceGradingOfMultipleAnswers(t *testing.T) {
	q := strings.Replace(choiceQuestion, "- {text: Deployment}", "- {text: Deployment, correct: true}", 1)
	q = strings.Replace(q, "stem:", "multiple: true\nstem:", 1)
	cat, err := catalog.Load(theoryFS(q))
	if err != nil {
		t.Fatal(err)
	}
	c := cat.Tracks["t"].Question("unit").Choice
	correct := c.Correct()
	if len(correct) != 2 {
		t.Fatalf("correct = %v", correct)
	}
	if !c.Grade(correct) || !c.Grade([]string{correct[1], correct[0]}) {
		t.Error("the full set must pass in any order")
	}
	if c.Grade(correct[:1]) {
		t.Error("a subset of the correct options must not pass")
	}
}

func TestChoiceRules(t *testing.T) {
	edit := func(old, new string) string { return strings.Replace(choiceQuestion, old, new, 1) }
	for name, c := range map[string]struct{ question, want string }{
		"no correct option":       {edit("{text: Pod, correct: true}", "{text: Pod}"), "want exactly 1"},
		"two correct, not multi":  {edit("{text: Node}", "{text: Node, correct: true}"), "want exactly 1"},
		"duplicate option":        {edit("{text: Node}", "{text: \" pod \"}"), "repeats another option"},
		"too few options":         {edit("  - {text: Deployment}\n  - {text: Node}\n", ""), "want 3 to 6"},
		"no explanation":          {edit("explanation: A Pod wraps one or more containers and is what the scheduler places.\n", ""), "explanation is required"},
		"no reference":            {edit("references: [{title: Pods, url: \"https://kubernetes.io/docs/concepts/workloads/pods/\"}]\n", ""), "reference is required"},
		"reference without https": {edit("https://kubernetes.io", "http://kubernetes.io"), "https url"},
		"unknown field":           {edit("stem:", "answer: Pod\nstem:"), "field answer not found"},
		"unknown domain":          {edit("domain: d", "domain: x"), "unknown domain"},
		"multiple with one right": {edit("stem:", "multiple: true\nstem:"), "at least 2 and not all"},
	} {
		wantErr(t, name, loadErr(theoryFS(c.question)), c.want)
	}
}

func TestChoiceWarnings(t *testing.T) {
	edit := func(old, new string) string { return strings.Replace(choiceQuestion, old, new, 1) }
	for name, c := range map[string]struct{ question, want string }{
		"refers to the other options": {edit("{text: Node}", "{text: None of the above}"), "random order"},
		"title gives the answer away": {edit("title: Smallest deployable unit", "title: The Pod is the smallest unit"), "title contains the correct answer"},
		"three options":               {edit("  - {text: Node}\n", ""), "the exams use 4"},
		"the long answer is right": {edit("{text: Pod, correct: true}",
			"{text: \"A Pod, which is a group of one or more containers with shared storage and network resources\", correct: true}"), "length gives it away"},
	} {
		cat, err := catalog.Load(theoryFS(c.question))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if w := strings.Join(cat.Tracks["t"].Question("unit").Warnings, "\n"); !strings.Contains(w, c.want) {
			t.Errorf("%s: want a warning containing %q, got: %q", name, c.want, w)
		}
	}
}

func TestQuestionShapeFollowsTrackKind(t *testing.T) {
	// A task directory in a theory track.
	fsys := theoryFS(choiceQuestion)
	fsys["t/questions/q/question.yaml"] = file("title: Q")
	wantErr(t, "task in theory track", loadErr(fsys), "choice questions only")

	// A choice file in a hands-on track.
	fsys = taskTrack(head+"kubernetesVersion: v1.37\nenvironments: {base: {}}\n",
		strings.NewReplacer("%s, %s", "exec, run: \"true\",").Replace(execQuestion))
	fsys["t/questions/unit.yaml"] = file(choiceQuestion)
	wantErr(t, "choice in task track", loadErr(fsys), "belong to a track with kind: theory")

	// A theory track has nothing to provision.
	fsys = theoryFS(choiceQuestion)
	fsys["t/track.yaml"] = file(theoryTrack + "environments: {base: {}}\n")
	wantErr(t, "theory with environments", loadErr(fsys), "has no environments")
}

func pathFS(pathYAML string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for _, id := range []string{"a", "b", "c"} {
		fsys[id+"/track.yaml"] = file(theoryTrack)
		fsys[id+"/lessons/d.md"] = file("# D")
		fsys[id+"/questions/unit.yaml"] = file(choiceQuestion)
	}
	if pathYAML != "" {
		fsys["path.yaml"] = file(pathYAML)
	}
	return fsys
}

func order(c *catalog.Catalog) string {
	var ids []string
	for _, t := range c.List() {
		ids = append(ids, t.ID)
	}
	return strings.Join(ids, " ")
}

func TestPathOrdersTracks(t *testing.T) {
	cat, err := catalog.Load(pathFS(""))
	if err != nil {
		t.Fatal(err)
	}
	if cat.Path != nil || order(cat) != "a b c" {
		t.Errorf("without path.yaml: path=%v order=%q, want none and alphabetical", cat.Path, order(cat))
	}

	cat, err = catalog.Load(pathFS("id: p\ntitle: P\nsteps:\n  - {track: c, role: foundation, why: First.}\n  - {track: a, role: required}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cat.Path == nil || len(cat.Path.Steps) != 2 {
		t.Fatalf("path = %+v", cat.Path)
	}
	// Path order first; a track outside the path stays listed, after it.
	if order(cat) != "c a b" {
		t.Errorf("order = %q, want %q", order(cat), "c a b")
	}
}

func TestPathRules(t *testing.T) {
	for name, c := range map[string]struct{ path, want string }{
		"unknown track":    {"id: p\ntitle: P\nsteps: [{track: x, role: required}]\n", "unknown track"},
		"track twice":      {"id: p\ntitle: P\nsteps: [{track: a, role: required}, {track: a, role: required}]\n", "appears twice"},
		"unknown role":     {"id: p\ntitle: P\nsteps: [{track: a, role: optional}]\n", "role must be"},
		"nothing required": {"id: p\ntitle: P\nsteps: [{track: a, role: foundation}]\n", "at least one step"},
		"no title":         {"id: p\nsteps: [{track: a, role: required}]\n", "id and title are required"},
		"unknown field":    {"id: p\ntitle: P\nlocked: true\nsteps: [{track: a, role: required}]\n", "field locked not found"},
	} {
		wantErr(t, name, loadErr(pathFS(c.path)), c.want)
	}
}
