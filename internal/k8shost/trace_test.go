package k8shost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/Mistral-valaise/kubedojo/internal/telemetry/teltest"
)

// Starting a sandbox Pod is two spans below the caller's: creating the Pod,
// and waiting for it. The wait is one span however often the Pod is looked
// at, with an event each time what it waits for changes.
func TestAcquireTracesTheCreationAndOneWait(t *testing.T) {
	a, p := newAPI(t, valid())
	creating := map[string]any{"phase": "Pending", "containerStatuses": []any{
		map[string]any{"state": map[string]any{"waiting": map[string]any{"reason": "ContainerCreating"}}}}}
	a.status = func(_ string, gets int) map[string]any {
		switch {
		case gets <= 2:
			return map[string]any{"phase": "Pending", "conditions": []any{condition("PodScheduled", "False", "Unschedulable", "0/3 nodes are available")}}
		case gets <= 5:
			return creating
		case gets == 6:
			return map[string]any{"phase": "Running"}
		}
		return readyStatus()
	}
	rec := teltest.New()
	ctx, parent := rec.Start(context.Background(), "sandbox.host.acquire")
	logf, _ := logs()
	if _, err := p.Acquire(ctx, "abc", logf); err != nil {
		t.Fatal(err)
	}
	parent.End(nil)

	if got, want := teltest.Shape(rec.Ended()), "sandbox.host.acquire\n   sandbox.pod.create\n   sandbox.pod.wait\n"; got != want {
		t.Fatalf("the trace has the shape\n%s\nwant\n%s", got, want)
	}
	t.Logf("acquire a sandbox Pod:\n%s", teltest.Tree(rec.Ended()))
	create, wait := rec.Named("sandbox.pod.create")[0], rec.Named("sandbox.pod.wait")[0]
	for _, s := range []struct {
		name  string
		attrs map[string]string
	}{{"create", stringAttrs(teltest.Attrs(create))}, {"wait", stringAttrs(teltest.Attrs(wait))}} {
		if s.attrs["k8s.pod.name"] != "kd-sandbox-abc" || s.attrs["k8s.namespace.name"] != "kubedojo" {
			t.Errorf("sandbox.pod.%s: %v", s.name, s.attrs)
		}
	}
	if teltest.Attrs(create)["http.response.status_code"].AsInt64() != 201 || teltest.Attrs(create)["container.image.name"].AsString() != "sandbox-host:v1" {
		t.Errorf("sandbox.pod.create: %v", create.Attributes())
	}
	// Seven looks: two unschedulable, three creating, one running, one ready.
	if n := teltest.Attrs(wait)["polls"].AsInt64(); n != 7 {
		t.Errorf("polls = %d, want 7", n)
	}
	var reasons []string
	for _, e := range teltest.Events(wait, "pod.state") {
		reasons = append(reasons, e["reason"].AsString()+"/"+e["k8s.pod.phase"].AsString())
	}
	if got, want := strings.Join(reasons, " "), "Unschedulable/Pending ContainerCreating/Pending Running/Running"; got != want {
		t.Errorf("events of the wait: %s, want %s", got, want)
	}

	// A Pod that is there already is neither created nor waited for.
	before := len(rec.Ended())
	if _, err := p.Acquire(ctx, "abc", logf); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.Ended()) - before; n != 0 {
		t.Errorf("%d spans for a sandbox that exists", n)
	}
}

func stringAttrs[V interface{ AsString() string }](in map[string]V) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v.AsString()
	}
	return out
}

// A Pod that cannot start fails the wait, with the last state as its event.
func TestAcquireTracesAWaitThatGivesUp(t *testing.T) {
	a, p := newAPI(t, valid())
	a.status = func(string, int) map[string]any {
		return map[string]any{"phase": "Failed", "reason": "Evicted", "message": "the node was low on disk"}
	}
	rec := teltest.New()
	ctx, parent := rec.Start(context.Background(), "sandbox.host.acquire")
	logf, _ := logs()
	if _, err := p.Acquire(ctx, "abc", logf); err == nil {
		t.Fatal("Acquire of a failed Pod succeeded")
	}
	parent.End(nil)
	wait := rec.Named("sandbox.pod.wait")[0]
	if wait.Status().Code != codes.Error || !strings.Contains(wait.Status().Description, "Evicted") {
		t.Errorf("status of the wait: %v %q", wait.Status().Code, wait.Status().Description)
	}
	if ev := teltest.Events(wait, "pod.state"); len(ev) != 1 || ev[0]["reason"].AsString() != "Failed" {
		t.Errorf("events of the wait: %v", ev)
	}
}

// The transport of the API client traces what changes something and nothing
// else: a GET happens for every command that runs in a sandbox.
func TestTransportTracesWritesOnly(t *testing.T) {
	var mu sync.Mutex
	var traceparent []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		traceparent = append(traceparent, r.Method+":"+r.Header.Get("Traceparent"))
		mu.Unlock()
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	rec := teltest.New()
	// The server sets the propagator for the whole process; here it is given.
	client := &http.Client{Transport: traced(srv.Client().Transport,
		otelhttp.WithTracerProvider(rec.Provider), otelhttp.WithPropagators(propagation.TraceContext{}))}
	p := newProvider(valid(), srv.URL, client, func() (string, error) { return "test-token", nil })
	p.cfg.Namespace = "kubedojo"

	ctx, parent := rec.Start(context.Background(), "work")
	if _, _, err := p.get(ctx, "kd-sandbox-abc"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.do(ctx, "POST", "/api/v1/namespaces/kubedojo/pods", map[string]string{"kind": "Pod"}); err != nil {
		t.Fatal(err)
	}
	if err := p.Release(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	parent.End(nil)

	want := "work\n   POST /api/v1/namespaces/{namespace}/pods\n   DELETE /api/v1/namespaces/{namespace}/pods/{name}\n"
	if got := teltest.Shape(rec.Ended()); got != want {
		t.Errorf("the trace has the shape\n%s\nwant\n%s", got, want)
	}
	for _, s := range rec.Ended() {
		if s.Name() != "work" && s.SpanKind() != trace.SpanKindClient {
			t.Errorf("%s is a %v span, want a client span", s.Name(), s.SpanKind())
		}
	}
	// A traced request carries its trace to the API server; a GET carries none.
	mu.Lock()
	defer mu.Unlock()
	if len(traceparent) != 3 || traceparent[0] != "GET:" || !strings.HasPrefix(traceparent[1], "POST:00-") || !strings.HasPrefix(traceparent[2], "DELETE:00-") {
		t.Errorf("traceparent headers: %v", traceparent)
	}
}

func TestRouteHidesNames(t *testing.T) {
	for in, want := range map[string]string{
		"/api/v1/namespaces/kubedojo/pods":                        "/api/v1/namespaces/{namespace}/pods",
		"/api/v1/namespaces/kubedojo/pods/kd-sandbox-abc":         "/api/v1/namespaces/{namespace}/pods/{name}",
		"/apis/apps/v1/namespaces/kubedojo/deployments/kubedojo":  "/apis/apps/v1/namespaces/{namespace}/deployments/{name}",
		"/api/v1/namespaces/pods/pods/pods":                       "/api/v1/namespaces/{namespace}/pods/{name}",
		"/healthz":                                                "/healthz",
		"/api/v1/namespaces/kubedojo/pods/kd-sandbox-abc/status/": "/api/v1/namespaces/{namespace}/pods/{name}/status/",
	} {
		if got := route(in); got != want {
			t.Errorf("route(%q) = %q, want %q", in, got, want)
		}
	}
}
