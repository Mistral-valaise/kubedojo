package session_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox/sandboxtest"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

const envTrackHead = "exam: {durationMinutes: 10, passPercent: 50, questions: 1}\ndomains: [{id: d, title: D, weight: 100}]\n"

func envQuestion(checks, flags string) string {
	return "title: Q\ndomain: d\ntask: do\nhints: [h]\n" + flags + "checks:\n" + checks
}

// envCatalog has two tracks that differ in their environment type only:
// "k" runs on a cluster, "l" on a Linux container.
func envCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	fsys := fstest.MapFS{}
	add := func(name, body string) { fsys[name] = &fstest.MapFile{Data: []byte(body)} }
	question := func(track, id, yaml string) {
		dir := track + "/questions/" + id + "/"
		add(dir+"question.yaml", yaml)
		add(dir+"setup.sh", "echo setup-"+id)
		add(dir+"cleanup.sh", "echo cleanup-"+id)
		add(dir+"solution.sh", "echo solution-"+id)
		add(dir+"solution.md", "do it")
	}
	check := func(id, on string) string {
		if on != "" {
			on = ", on: " + on
		}
		return "  - {id: " + id + ", title: T, type: exec, run: echo check-" + id + ", explain: e" + on + "}\n"
	}
	add("k/track.yaml", "title: K\nkubernetesVersion: v1.37\n"+envTrackHead+"environments: {base: {workers: 1}}\n")
	add("k/lessons/d.md", "# D")
	question("k", "q", envQuestion(check("main", "")+check("worker", "worker")+check("term", "terminal"), ""))

	add("l/track.yaml", "title: L\nkind: skills\n"+envTrackHead+"environments: {base: {type: container}}\n")
	add("l/lessons/d.md", "# D")
	question("l", "q", envQuestion(check("main", "")+check("term", "terminal"), ""))
	question("l", "fresh", envQuestion(check("main", ""), "freshEnvironment: true\n"))

	cat, err := catalog.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func envManager(t *testing.T) (*session.Manager, *sandboxtest.Driver) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	drv := &sandboxtest.Driver{}
	mgr, err := session.NewManager(session.Config{Catalog: envCatalog(t), Store: st, Driver: drv})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Close)
	return mgr, drv
}

// prepare makes a question ready and returns the alias of its environment.
func prepare(t *testing.T, mgr *session.Manager, sid, qid string, reset bool) string {
	t.Helper()
	if err := mgr.Prepare(sid, qid, reset); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		v, err := mgr.View(sid)
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range v.Questions {
			if q.ID == qid && q.Status == "ready" {
				return v.Envs["base"]
			}
			if q.ID == qid && q.Status == "error" {
				t.Fatalf("question %s: %s", qid, q.Error)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("question %s never became ready", qid)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// ran returns the one recorded call whose script contains marker, looking
// only at calls made after the first skip ones.
func ran(t *testing.T, drv *sandboxtest.Driver, skip int, marker string) sandboxtest.ExecCall {
	t.Helper()
	var found []sandboxtest.ExecCall
	for _, c := range drv.Execs()[skip:] {
		if strings.Contains(c.Script, marker) {
			found = append(found, c)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%q ran %d times, want once", marker, len(found))
	}
	return found[0]
}

// Where a script runs and as whom follows from the environment of its
// question, never from a name in the session code.
func TestScriptsRunOnTheRightMachineAsTheRightUser(t *testing.T) {
	type want struct{ role, user string }
	for track, c := range map[string]struct {
		envType    string
		setup      want
		checks     map[string]want
		solve      want
		kubeconfig bool
	}{
		"k": {sandbox.EnvCluster, want{"control-plane", "root"},
			map[string]want{"main": {"control-plane", "root"}, "worker": {"worker", "root"}, "term": {"terminal", "candidate"}},
			want{"control-plane", "root"}, true},
		"l": {sandbox.EnvContainer, want{"host", "root"},
			map[string]want{"main": {"host", "root"}, "term": {"terminal", "candidate"}},
			want{"host", "candidate"}, false},
	} {
		mgr, drv := envManager(t)
		ses, err := mgr.Practice(track, "")
		if err != nil {
			t.Fatal(err)
		}
		alias := prepare(t, mgr, ses.ID, "q", false)

		env := sandbox.Env{Alias: alias, Type: c.envType}
		if got := drv.Created(); len(got) != 1 || got[0] != env {
			t.Errorf("%s: created %+v, want %+v", track, got, env)
		}
		if got := drv.Wired(); len(got) != 1 || got[0] != env {
			t.Errorf("%s: terminal wired to %+v, want %+v", track, got, env)
		}

		is := func(what string, call sandboxtest.ExecCall, w want) {
			t.Helper()
			if call.Target.Role != w.role || call.User != w.user || call.Target.Alias != alias {
				t.Errorf("%s: %s ran on %s/%s as %s, want %s/%s as %s", track, what,
					call.Target.Alias, call.Target.Role, call.User, alias, w.role, w.user)
			}
			if call.Env["KD_HOST"] != alias || call.Env["KD_QUESTION"] != "q" {
				t.Errorf("%s: %s got KD_HOST=%q KD_QUESTION=%q", track, what, call.Env["KD_HOST"], call.Env["KD_QUESTION"])
			}
			// kubectl's admin kubeconfig belongs to cluster nodes only.
			_, has := call.Env["KUBECONFIG"]
			if wantKC := c.kubeconfig && w.role != sandbox.RoleTerminal; has != wantKC {
				t.Errorf("%s: %s has KUBECONFIG = %v, want %v", track, what, has, wantKC)
			}
		}
		is("setup.sh", ran(t, drv, 0, "setup-q"), c.setup)

		n := len(drv.Execs())
		if _, err := mgr.Check(context.Background(), ses.ID, "q"); err != nil {
			t.Fatal(err)
		}
		for id, w := range c.checks {
			is("check "+id, ran(t, drv, n, "check-"+id), w)
		}

		n = len(drv.Execs())
		if _, err := mgr.Solve(context.Background(), ses.ID, "q", "echo variant"); err != nil {
			t.Fatal(err)
		}
		is("a solution", ran(t, drv, n, "variant"), c.solve)

		// Exec without a role is how the API asks for "the main machine".
		n = len(drv.Execs())
		if _, err := mgr.Exec(context.Background(), ses.ID, "q", "", "echo snapshot"); err != nil {
			t.Fatal(err)
		}
		is("a script without a role", ran(t, drv, n, "snapshot"), c.setup)

		n = len(drv.Execs())
		prepare(t, mgr, ses.ID, "q", true)
		is("cleanup.sh", ran(t, drv, n, "cleanup-q"), c.setup)
		is("setup.sh on reset", ran(t, drv, n, "setup-q"), c.setup)

		if err := mgr.Destroy(context.Background(), ses.ID); err != nil {
			t.Fatal(err)
		}
		if len(drv.DestroyedEnvs) != 1 || len(drv.DestroyedEnvs[0]) != 1 || drv.DestroyedEnvs[0][0] != env {
			t.Errorf("%s: destroyed %+v, want %+v", track, drv.DestroyedEnvs, env)
		}
	}
}

// Resetting a freshEnvironment question on a Linux host throws the container
// away and starts a new one, as it does with a cluster.
func TestFreshEnvironmentRecreatesTheHost(t *testing.T) {
	mgr, drv := envManager(t)
	ses, err := mgr.Practice("l", "")
	if err != nil {
		t.Fatal(err)
	}
	first := prepare(t, mgr, ses.ID, "fresh", false)
	prepare(t, mgr, ses.ID, "q", false)
	second := prepare(t, mgr, ses.ID, "fresh", true)

	if first == second {
		t.Fatalf("the host kept its name %s: it was not recreated", first)
	}
	if got, want := drv.Deleted(), (sandbox.Env{Alias: first, Type: sandbox.EnvContainer}); len(got) != 1 || got[0] != want {
		t.Errorf("deleted %+v, want %+v", got, want)
	}
	if got, want := drv.Wired(), (sandbox.Env{Alias: second, Type: sandbox.EnvContainer}); len(got) != 1 || got[0] != want {
		t.Errorf("terminal wired to %+v, want %+v", got, want)
	}
	// No cleanup.sh: the old host is gone, there is nothing to clean.
	for _, c := range drv.Execs() {
		if strings.Contains(c.Script, "cleanup-fresh") {
			t.Error("cleanup.sh ran although the host was recreated")
		}
	}
	// The other question lived on the old host and has to be set up again.
	v, _ := mgr.View(ses.ID)
	for _, q := range v.Questions {
		if q.ID == "q" && q.Status != "pending" {
			t.Errorf("question q is %s on a host it was never set up on", q.Status)
		}
	}
}
