package k8shost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// api is a Kubernetes API server that knows Pods of one Namespace and one
// Deployment. Pods do not run; the test decides what their status says.
type api struct {
	mu      sync.Mutex
	pods    map[string]map[string]any
	created []map[string]any
	deleted []string
	listed  []string // label selectors of list requests
	// status is called for every GET of a Pod and returns its status.
	status func(name string, gets int) map[string]any
	gets   map[string]int
}

func condition(typ, status, reason, message string) map[string]any {
	return map[string]any{"type": typ, "status": status, "reason": reason, "message": message}
}

func readyStatus() map[string]any {
	return map[string]any{"phase": "Running", "podIP": "10.42.0.7", "conditions": []any{condition("Ready", "True", "", "")}}
}

func (a *api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer test-token" {
		w.WriteHeader(401)
		return
	}
	const pods = "/api/v1/namespaces/kubedojo/pods"
	switch {
	case r.URL.Path == "/apis/apps/v1/namespaces/kubedojo/deployments/kubedojo" && r.Method == "GET":
		_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"uid": "deploy-uid-1"}})
	case r.URL.Path == pods && r.Method == "POST":
		var p map[string]any
		_ = json.NewDecoder(r.Body).Decode(&p)
		name := p["metadata"].(map[string]any)["name"].(string)
		if _, ok := a.pods[name]; ok {
			w.WriteHeader(409)
			return
		}
		p["metadata"].(map[string]any)["creationTimestamp"] = time.Now().UTC().Format(time.RFC3339)
		a.pods[name] = p
		a.created = append(a.created, p)
		w.WriteHeader(201)
	case r.URL.Path == pods && r.Method == "GET":
		a.listed = append(a.listed, r.URL.Query().Get("labelSelector"))
		items := []any{}
		for _, p := range a.pods {
			items = append(items, p)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	case strings.HasPrefix(r.URL.Path, pods+"/") && r.Method == "GET":
		name := strings.TrimPrefix(r.URL.Path, pods+"/")
		p, ok := a.pods[name]
		if !ok {
			w.WriteHeader(404)
			return
		}
		a.gets[name]++
		out := map[string]any{"metadata": p["metadata"], "status": a.status(name, a.gets[name])}
		_ = json.NewEncoder(w).Encode(out)
	case strings.HasPrefix(r.URL.Path, pods+"/") && r.Method == "DELETE":
		name := strings.TrimPrefix(r.URL.Path, pods+"/")
		if _, ok := a.pods[name]; !ok {
			w.WriteHeader(404)
			return
		}
		delete(a.pods, name)
		a.deleted = append(a.deleted, name)
	default:
		w.WriteHeader(500)
	}
}

func newAPI(t *testing.T, cfg Config) (*api, *Provider) {
	t.Helper()
	a := &api{pods: map[string]map[string]any{}, gets: map[string]int{},
		status: func(string, int) map[string]any { return readyStatus() }}
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	cfg.Namespace = "kubedojo"
	p := newProvider(cfg, srv.URL, srv.Client(), func() (string, error) { return "test-token", nil })
	p.poll = time.Millisecond
	return a, p
}

func logs() (func(string, ...any), *[]string) {
	var lines []string
	return func(format string, a ...any) { lines = append(lines, format) }, &lines
}

func TestAcquireCreatesThePodOnceAndWaitsForIt(t *testing.T) {
	cfg := valid()
	cfg.Instance, cfg.OwnerDeployment = "rel", "kubedojo"
	a, p := newAPI(t, cfg)
	// Ready on the third look.
	a.status = func(_ string, gets int) map[string]any {
		if gets < 3 {
			return map[string]any{"phase": "Pending"}
		}
		return readyStatus()
	}
	if err := p.AdoptOwner(context.Background()); err != nil {
		t.Fatal(err)
	}
	logf, _ := logs()
	host, err := p.Acquire(context.Background(), "abc", logf)
	if err != nil {
		t.Fatal(err)
	}
	if host.Endpoint != "tcp://10.42.0.7:2375" {
		t.Errorf("endpoint %q", host.Endpoint)
	}
	// A second Acquire finds the Pod.
	if _, err := p.Acquire(context.Background(), "abc", logf); err != nil {
		t.Fatal(err)
	}
	if len(a.created) != 1 {
		t.Fatalf("%d Pods were created for one session", len(a.created))
	}
	meta := a.created[0]["metadata"].(map[string]any)
	labels := meta["labels"].(map[string]any)
	if labels["app.kubernetes.io/instance"] != "rel" || labels[sessionLabel] != "abc" || labels["app.kubernetes.io/component"] != "sandbox" {
		t.Errorf("labels: %v", labels)
	}
	// Owned by the release, so that an uninstall takes the sandbox along.
	owners, _ := meta["ownerReferences"].([]any)
	if len(owners) != 1 {
		t.Fatalf("ownerReferences: %v", meta["ownerReferences"])
	}
	o := owners[0].(map[string]any)
	if o["kind"] != "Deployment" || o["name"] != "kubedojo" || o["uid"] != "deploy-uid-1" || o["apiVersion"] != "apps/v1" {
		t.Errorf("owner: %v", o)
	}
}

// Lookup is for work that needs an existing sandbox. It must never create one.
func TestLookupNeverCreates(t *testing.T) {
	a, p := newAPI(t, valid())
	_, ok, err := p.Lookup(context.Background(), "gone")
	if err != nil || ok {
		t.Fatalf("Lookup of a missing Pod = %v, %v", ok, err)
	}
	if len(a.created) != 0 {
		t.Fatal("Lookup created a Pod")
	}
	logf, _ := logs()
	if _, err := p.Acquire(context.Background(), "abc", logf); err != nil {
		t.Fatal(err)
	}
	host, ok, err := p.Lookup(context.Background(), "abc")
	if err != nil || !ok || host.Endpoint == "" {
		t.Fatalf("Lookup of a ready Pod = %+v, %v, %v", host, ok, err)
	}
	// A Pod that exists but is not ready has no Docker to talk to.
	a.status = func(string, int) map[string]any { return map[string]any{"phase": "Pending"} }
	if _, ok, _ := p.Lookup(context.Background(), "abc"); ok {
		t.Fatal("Lookup returned a Pod that is not ready")
	}
}

// A Pod that cannot be scheduled or whose image cannot be pulled does not
// become ready by waiting. The learner hears why, and soon.
func TestAcquireExplainsAPodThatCannotStart(t *testing.T) {
	for name, c := range map[string]struct {
		status map[string]any
		want   string
	}{
		"unschedulable": {map[string]any{"phase": "Pending", "conditions": []any{
			condition("PodScheduled", "False", "Unschedulable", "0/1 nodes are available: 1 Insufficient memory.")}}, "Insufficient memory"},
		"image": {map[string]any{"phase": "Pending", "containerStatuses": []any{map[string]any{"state": map[string]any{
			"waiting": map[string]any{"reason": "ImagePullBackOff", "message": "Back-off pulling image \"sandbox-host:v1\""}}}}}, "cannot be pulled"},
		"evicted": {map[string]any{"phase": "Failed", "reason": "Evicted", "message": "The node was low on resource: ephemeral-storage."}, "ephemeral-storage"},
	} {
		t.Run(name, func(t *testing.T) {
			a, p := newAPI(t, valid())
			a.status = func(string, int) map[string]any { return c.status }
			p.patience = map[string]time.Duration{waitUnschedulable: 20 * time.Millisecond, waitImage: 20 * time.Millisecond}
			logf, lines := logs()
			start := time.Now()
			_, err := p.Acquire(context.Background(), "abc", logf)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Acquire = %v, want an error with %q", err, c.want)
			}
			if time.Since(start) > 3*time.Second {
				t.Fatalf("Acquire took %s to give up", time.Since(start))
			}
			if name != "evicted" && len(*lines) < 2 {
				t.Errorf("the reason was not written to the session log: %v", *lines)
			}
		})
	}
}

func TestAcquireTimesOutWithTheLastReason(t *testing.T) {
	a, p := newAPI(t, valid())
	a.status = func(string, int) map[string]any {
		return map[string]any{"phase": "Pending", "containerStatuses": []any{map[string]any{"state": map[string]any{
			"waiting": map[string]any{"reason": "CreateContainerConfigError", "message": "secret not found"}}}}}
	}
	p.startTimeout = 30 * time.Millisecond
	logf, _ := logs()
	_, err := p.Acquire(context.Background(), "abc", logf)
	if err == nil || !strings.Contains(err.Error(), "not ready after") || !strings.Contains(err.Error(), "secret not found") {
		t.Fatalf("Acquire = %v, want a timeout that says what the Pod waits for", err)
	}
	// The context of the caller ends the wait as well.
	p.startTimeout = time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := p.Acquire(ctx, "abc", logf); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire with a cancelled context = %v", err)
	}
}

// Reconcile removes the Pods of sessions that no longer exist: only those of
// this installation, and none that was just created.
func TestReconcileRemovesOrphans(t *testing.T) {
	cfg := valid()
	cfg.Instance = "rel"
	a, p := newAPI(t, cfg)
	logf, _ := logs()
	for _, id := range []string{"live", "orphan", "fresh"} {
		if _, err := p.Acquire(context.Background(), id, logf); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	a.mu.Lock()
	a.pods[podName("live")]["metadata"].(map[string]any)["creationTimestamp"] = old
	a.pods[podName("orphan")]["metadata"].(map[string]any)["creationTimestamp"] = old
	a.mu.Unlock()

	removed, err := p.Reconcile(context.Background(), func(id string) bool { return id == "live" })
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "orphan" {
		t.Fatalf("removed %v, want only the orphan", removed)
	}
	if len(a.deleted) != 1 || a.deleted[0] != podName("orphan") {
		t.Fatalf("deleted %v", a.deleted)
	}
	// The list is limited to this installation's sandboxes.
	if len(a.listed) != 1 || !strings.Contains(a.listed[0], "app.kubernetes.io/instance=rel") || !strings.Contains(a.listed[0], "app.kubernetes.io/component=sandbox") {
		t.Fatalf("label selector %q", a.listed)
	}
}

func TestReleaseOfAMissingPodIsNotAnError(t *testing.T) {
	_, p := newAPI(t, valid())
	if err := p.Release(context.Background(), "never-existed"); err != nil {
		t.Fatal(err)
	}
}
