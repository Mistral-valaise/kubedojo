// Package k8shost runs each session's sandbox in its own Kubernetes Pod: a
// privileged Docker-in-Docker daemon that kubedojo then uses exactly like a
// local Docker endpoint. It talks to the API server with the Pod's own
// ServiceAccount and needs only create/get/delete on Pods in one Namespace.
package k8shost

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
	"github.com/Mistral-valaise/kubedojo/internal/span"
)

const saDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// Config describes the sandbox Pods.
type Config struct {
	Namespace string // where sandbox Pods are created
	Image     string // Docker-in-Docker image with the sandbox images preloaded
	// Resources of one sandbox, as Kubernetes quantities. Requests are what
	// the scheduler reserves on a node, limits are what a sandbox may use at
	// most. See Validate for what is accepted.
	CPURequest string // e.g. "1"
	CPULimit   string // e.g. "4"; empty for none
	MemRequest string // e.g. "3Gi"
	MemLimit   string // e.g. "8Gi"; empty for none
	// Ephemeral storage is the node's disk: the sandbox's Docker data
	// (images, the clusters' container filesystems) lives in an emptyDir.
	// Above the limit the kubelet evicts the Pod, which ends the session.
	EphemeralRequest string            // e.g. "10Gi"
	EphemeralLimit   string            // e.g. "30Gi"; empty for none
	NodeSelector     map[string]string // dedicated node pool
	Toleration       string            // taint key tolerated by sandbox Pods (effect NoSchedule)
	PullSecret       string
	// Instance is the name of the installation (the Helm release). Sandbox
	// Pods carry it as a label, so that two installations in one Namespace
	// do not clean up or reach each other's sandboxes.
	Instance string
	// OwnerDeployment names the server's Deployment. Sandbox Pods get it as
	// owner: when the installation is removed, Kubernetes removes them too.
	OwnerDeployment string
	// StartTimeout is how long a sandbox Pod may take to become ready (default 8m).
	StartTimeout time.Duration
}

const sessionLabel = "kubedojo.dev/session"

// ProbePrefix starts the session ID of a sandbox that `kubedojo doctor
// --hosted` creates to prove that sandboxes work.
const ProbePrefix = "probe"

type Provider struct {
	cfg    Config
	api    string
	client *http.Client
	// token returns the bearer token of a request.
	token func() (string, error)

	// ownerUID is the UID of cfg.OwnerDeployment, empty if it is unknown.
	ownerUID string
	poll     time.Duration
	// patience is how long a Pod may stay in a state that does not resolve
	// on its own before Acquire gives up.
	patience     map[string]time.Duration
	startTimeout time.Duration
	// orphanAge is how old a Pod without a session must be to be removed.
	orphanAge time.Duration
}

// newProvider is New without the in-cluster discovery.
func newProvider(cfg Config, api string, client *http.Client, token func() (string, error)) *Provider {
	p := &Provider{cfg: cfg, api: api, client: client, token: token, poll: 3 * time.Second,
		// Unschedulable can pass when another session ends, but a learner
		// should not stare at a spinner for eight minutes to find out.
		patience:     map[string]time.Duration{waitUnschedulable: time.Minute, waitImage: 2 * time.Minute},
		startTimeout: cfg.StartTimeout, orphanAge: 2 * time.Minute}
	if p.startTimeout <= 0 {
		p.startTimeout = 8 * time.Minute
	}
	return p
}

// New builds a provider from the in-cluster ServiceAccount.
func New(cfg Config) (*Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" {
		return nil, fmt.Errorf("sandbox host \"kubernetes\" only works inside a cluster (KUBERNETES_SERVICE_HOST is not set)")
	}
	ca, err := os.ReadFile(saDir + "/ca.crt")
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca)
	if cfg.Namespace == "" {
		ns, _ := os.ReadFile(saDir + "/namespace")
		cfg.Namespace = strings.TrimSpace(string(ns))
	}
	return newProvider(cfg, "https://"+net.JoinHostPort(host, port),
		&http.Client{Timeout: 30 * time.Second, Transport: traced(&http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}})},
		func() (string, error) {
			b, err := os.ReadFile(saDir + "/token") // re-read: projected tokens rotate
			return strings.TrimSpace(string(b)), err
		}), nil
}

// AdoptOwner looks up the server's Deployment, so that sandbox Pods created
// from now on are owned by it. Without it they still work; they are then
// only cleaned up by Reconcile and by hand.
func (p *Provider) AdoptOwner(ctx context.Context) error {
	if p.cfg.OwnerDeployment == "" {
		return nil
	}
	code, body, err := p.do(ctx, "GET", "/apis/apps/v1/namespaces/"+p.cfg.Namespace+"/deployments/"+p.cfg.OwnerDeployment, nil)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("get deployment %s: HTTP %d: %s", p.cfg.OwnerDeployment, code, apiMessage(body))
	}
	var d struct {
		Metadata struct {
			UID string `json:"uid"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return err
	}
	p.ownerUID = d.Metadata.UID
	return nil
}

// Namespace is where the sandbox Pods are created.
func (p *Provider) Namespace() string { return p.cfg.Namespace }

func podName(sessionID string) string { return "kd-sandbox-" + sessionID }

func (p *Provider) do(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.api+path, rd)
	if err != nil {
		return 0, nil, err
	}
	token, err := p.token()
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	return resp.StatusCode, out, err
}

type podStatus struct {
	Metadata struct {
		Name              string            `json:"name"`
		CreationTimestamp time.Time         `json:"creationTimestamp"`
		Labels            map[string]string `json:"labels"`
	} `json:"metadata"`
	Status struct {
		Phase      string `json:"phase"`
		PodIP      string `json:"podIP"`
		Reason     string `json:"reason"`
		Message    string `json:"message"`
		Conditions []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"conditions"`
		ContainerStatuses []struct {
			State struct {
				Waiting *struct {
					Reason  string `json:"reason"`
					Message string `json:"message"`
				} `json:"waiting"`
			} `json:"state"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

func (p *Provider) get(ctx context.Context, name string) (*podStatus, int, error) {
	code, body, err := p.do(ctx, "GET", "/api/v1/namespaces/"+p.cfg.Namespace+"/pods/"+name, nil)
	if err != nil || code != 200 {
		return nil, code, err
	}
	st := &podStatus{}
	return st, code, json.Unmarshal(body, st)
}

func ready(st *podStatus) bool {
	for _, c := range st.Status.Conditions {
		if c.Type == "Ready" && c.Status == "True" {
			return st.Status.PodIP != ""
		}
	}
	return false
}

// Why a Pod is not ready yet, in the order a learner would want to hear it.
const (
	waitUnschedulable = "unschedulable"
	waitImage         = "image"
)

// waiting explains a Pod that is not ready: kind is one of the wait
// constants for states that will not resolve on their own, text is for the
// learner and the operator.
func waiting(st *podStatus) (kind, text string) {
	for _, c := range st.Status.Conditions {
		if c.Type == "PodScheduled" && c.Status == "False" && c.Reason == "Unschedulable" {
			return waitUnschedulable, "no node has room for the sandbox: " + c.Message
		}
	}
	for _, cs := range st.Status.ContainerStatuses {
		if w := cs.State.Waiting; w != nil {
			switch w.Reason {
			case "ImagePullBackOff", "ErrImagePull", "InvalidImageName", "ErrImageNeverPull":
				return waitImage, "the sandbox image cannot be pulled (" + w.Reason + "): " + w.Message
			case "ContainerCreating", "PodInitializing", "":
				// The normal path; nothing to say.
			default:
				return "", "the sandbox container is waiting (" + w.Reason + "): " + w.Message
			}
		}
	}
	return "", ""
}

func (p *Provider) endpoint(st *podStatus) dockerhost.Host {
	return dockerhost.Host{Endpoint: "tcp://" + net.JoinHostPort(st.Status.PodIP, "2375")}
}

// Lookup returns the Docker endpoint of the session's sandbox Pod if the Pod
// exists and is ready. Unlike Acquire it never creates one.
func (p *Provider) Lookup(ctx context.Context, sessionID string) (dockerhost.Host, bool, error) {
	st, code, err := p.get(ctx, podName(sessionID))
	if err != nil {
		return dockerhost.Host{}, false, err
	}
	switch {
	case code == 404:
		return dockerhost.Host{}, false, nil
	case code != 200:
		return dockerhost.Host{}, false, fmt.Errorf("get sandbox pod: HTTP %d", code)
	case !ready(st):
		return dockerhost.Host{}, false, nil
	}
	return p.endpoint(st), true, nil
}

// Acquire returns the Docker endpoint of the session's sandbox Pod, creating
// the Pod on first use. While the Pod is not ready it tells logf why, and it
// gives up early on states that do not resolve on their own: a Pod no node
// has room for, an image that cannot be pulled.
func (p *Provider) Acquire(ctx context.Context, sessionID string, logf func(string, ...any)) (dockerhost.Host, error) {
	name := podName(sessionID)
	st, code, err := p.get(ctx, name)
	if err != nil {
		return dockerhost.Host{}, err
	}
	if code == 200 && ready(st) {
		return p.endpoint(st), nil
	}
	pod := []span.Attr{span.String("k8s.pod.name", name), span.String("k8s.namespace.name", p.cfg.Namespace)}
	if code == 404 {
		logf("Starting the sandbox host ...")
		if err := p.create(ctx, name, sessionID, pod); err != nil {
			return dockerhost.Host{}, err
		}
	} else if code != 200 {
		return dockerhost.Host{}, fmt.Errorf("get sandbox pod: HTTP %d", code)
	}
	// Waiting is one span, however often the Pod is looked at: the number of
	// looks is an attribute, and each change of what the Pod waits for is an
	// event.
	ctx, sp := span.Start(ctx, "sandbox.pod.wait", pod...)
	polls := 0
	host, err := p.wait(ctx, name, logf, sp, &polls)
	sp.Set(span.Int("polls", polls))
	sp.End(err)
	return host, err
}

// create creates the sandbox Pod of a session. A Pod that exists already
// (another request was faster) is as good.
func (p *Provider) create(ctx context.Context, name, sessionID string, pod []span.Attr) (err error) {
	ctx, sp := span.Start(ctx, "sandbox.pod.create", append(pod, span.String("container.image.name", p.cfg.Image))...)
	defer func() { sp.End(err) }()
	c, body, err := p.do(ctx, "POST", "/api/v1/namespaces/"+p.cfg.Namespace+"/pods", p.manifest(name, sessionID))
	if err != nil {
		return err
	}
	sp.Set(span.Int("http.response.status_code", c))
	if c != 201 && c != 409 {
		return fmt.Errorf("create sandbox pod: HTTP %d: %s", c, apiMessage(body))
	}
	return nil
}

// state says in a word or two where a Pod that is not ready stands: what it
// waits for (Unschedulable, ContainerCreating, ImagePullBackOff, ...), or
// else its phase.
func state(st *podStatus) string {
	for _, c := range st.Status.Conditions {
		if c.Type == "PodScheduled" && c.Status == "False" && c.Reason != "" {
			return c.Reason
		}
	}
	for _, cs := range st.Status.ContainerStatuses {
		if w := cs.State.Waiting; w != nil && w.Reason != "" {
			return w.Reason
		}
	}
	return firstOf(st.Status.Phase, "Unknown")
}

// wait polls the sandbox Pod until it is ready, tells logf why it is not, and
// gives up early on states that do not resolve on their own.
func (p *Provider) wait(ctx context.Context, name string, logf func(string, ...any), sp span.Span, polls *int) (dockerhost.Host, error) {
	start := time.Now()
	said, stuck, since, was := "", "", time.Time{}, ""
	for {
		*polls++
		st, code, err := p.get(ctx, name)
		if err != nil {
			return dockerhost.Host{}, err
		}
		if code == 404 {
			return dockerhost.Host{}, fmt.Errorf("sandbox pod %s was removed while it was starting", name)
		}
		if code == 200 {
			if ready(st) {
				logf("Sandbox host is ready.")
				return p.endpoint(st), nil
			}
			if now := state(st); now != was {
				sp.Event("pod.state", span.String("reason", now), span.String("k8s.pod.phase", st.Status.Phase))
				was = now
			}
			if ph := st.Status.Phase; ph == "Failed" || ph == "Succeeded" {
				return dockerhost.Host{}, fmt.Errorf("sandbox pod %s ended (%s): %s", name, firstOf(st.Status.Reason, ph), firstOf(st.Status.Message, "see kubectl describe pod "+name))
			}
			kind, text := waiting(st)
			if text != "" && text != said {
				logf("Sandbox host: %s", text)
				said = text
			}
			if kind != stuck {
				stuck, since = kind, time.Now()
			}
			if limit := p.patience[kind]; kind != "" && limit > 0 && time.Since(since) > limit {
				return dockerhost.Host{}, fmt.Errorf("the sandbox cannot start: %s", text)
			}
		}
		if time.Since(start) > p.startTimeout {
			_, text := waiting(st)
			return dockerhost.Host{}, fmt.Errorf("sandbox pod %s was not ready after %s%s", name, p.startTimeout, prefixed(": ", text))
		}
		select {
		case <-ctx.Done():
			return dockerhost.Host{}, ctx.Err()
		case <-time.After(p.poll):
		}
	}
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func prefixed(prefix, s string) string {
	if s == "" {
		return ""
	}
	return prefix + s
}

// apiMessage is the message of a Kubernetes Status object, or the raw body.
func apiMessage(body []byte) string {
	var st struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &st) == nil && st.Message != "" {
		return st.Message
	}
	return strings.TrimSpace(string(body))
}

// Release deletes the session's sandbox Pod; everything inside goes with it.
func (p *Provider) Release(ctx context.Context, sessionID string) error {
	return p.delete(ctx, podName(sessionID))
}

func (p *Provider) delete(ctx context.Context, name string) error {
	code, body, err := p.do(ctx, "DELETE", "/api/v1/namespaces/"+p.cfg.Namespace+"/pods/"+name+"?gracePeriodSeconds=5", nil)
	if err != nil {
		return err
	}
	if code != 200 && code != 202 && code != 404 {
		return fmt.Errorf("delete sandbox pod: HTTP %d: %s", code, apiMessage(body))
	}
	return nil
}

// selector matches the sandbox Pods of this installation, and no others in
// the Namespace.
func (p *Provider) selector() string {
	sel := "app.kubernetes.io/name=kubedojo,app.kubernetes.io/component=sandbox"
	if p.cfg.Instance != "" {
		sel += ",app.kubernetes.io/instance=" + p.cfg.Instance
	}
	return sel
}

// Reconcile deletes sandbox Pods that belong to no live session: left behind
// by a server that was killed, by a session that was removed from the
// database, or by a failed deletion. A Pod younger than two minutes is left
// alone, because its session may be about to be recorded. It returns the
// sessions whose Pods were removed.
func (p *Provider) Reconcile(ctx context.Context, live func(sessionID string) bool) ([]string, error) {
	code, body, err := p.do(ctx, "GET", "/api/v1/namespaces/"+p.cfg.Namespace+"/pods?labelSelector="+url.QueryEscape(p.selector()), nil)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("list sandbox pods: HTTP %d: %s", code, apiMessage(body))
	}
	var list struct {
		Items []podStatus `json:"items"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	var removed []string
	for _, pod := range list.Items {
		id := pod.Metadata.Labels[sessionLabel]
		age := time.Since(pod.Metadata.CreationTimestamp)
		if id == "" || live(id) || age < p.orphanAge {
			continue
		}
		// The probe of `kubedojo doctor --hosted` is no session of the
		// server. It removes its Pod itself; only a forgotten one is cleaned up.
		if strings.HasPrefix(id, ProbePrefix) && age < 30*time.Minute {
			continue
		}
		if err := p.delete(ctx, pod.Metadata.Name); err != nil {
			return removed, err
		}
		removed = append(removed, id)
	}
	return removed, nil
}

func (p *Provider) manifest(name, sessionID string) map[string]any {
	requests := map[string]string{"cpu": p.cfg.CPURequest, "memory": p.cfg.MemRequest, "ephemeral-storage": p.cfg.EphemeralRequest}
	limits := map[string]string{}
	for k, v := range map[string]string{"cpu": p.cfg.CPULimit, "memory": p.cfg.MemLimit, "ephemeral-storage": p.cfg.EphemeralLimit} {
		if v != "" {
			limits[k] = v
		}
	}
	res := map[string]any{"requests": requests}
	if len(limits) > 0 {
		res["limits"] = limits
	}
	// Docker's data is all of the sandbox's disk use. The size limit makes
	// the kubelet evict a sandbox that fills it, before the node's disk is full.
	docker := map[string]any{}
	if p.cfg.EphemeralLimit != "" {
		docker["sizeLimit"] = p.cfg.EphemeralLimit
	}
	spec := map[string]any{
		"restartPolicy":                 "Never",
		"automountServiceAccountToken":  false,
		"enableServiceLinks":            false,
		"terminationGracePeriodSeconds": 10,
		"containers": []any{map[string]any{
			"name":            "dind",
			"image":           p.cfg.Image,
			"securityContext": map[string]any{"privileged": true},
			"resources":       res,
			"ports":           []any{map[string]any{"name": "docker", "containerPort": 2375}},
			"readinessProbe": map[string]any{
				"exec":          map[string]any{"command": []string{"test", "-f", "/run/kubedojo-ready"}},
				"periodSeconds": 3, "failureThreshold": 200,
			},
			"volumeMounts": []any{
				map[string]any{"name": "docker", "mountPath": "/var/lib/docker"},
				map[string]any{"name": "modules", "mountPath": "/lib/modules", "readOnly": true},
			},
		}},
		"volumes": []any{
			map[string]any{"name": "docker", "emptyDir": docker},
			map[string]any{"name": "modules", "hostPath": map[string]any{"path": "/lib/modules"}},
		},
	}
	if len(p.cfg.NodeSelector) > 0 {
		spec["nodeSelector"] = p.cfg.NodeSelector
	}
	if p.cfg.Toleration != "" {
		spec["tolerations"] = []any{map[string]any{"key": p.cfg.Toleration, "operator": "Exists", "effect": "NoSchedule"}}
	}
	if p.cfg.PullSecret != "" {
		spec["imagePullSecrets"] = []any{map[string]string{"name": p.cfg.PullSecret}}
	}
	labels := map[string]string{
		"app.kubernetes.io/name": "kubedojo", "app.kubernetes.io/component": "sandbox",
		sessionLabel: sessionID,
	}
	if p.cfg.Instance != "" {
		labels["app.kubernetes.io/instance"] = p.cfg.Instance
	}
	meta := map[string]any{"name": name, "labels": labels}
	if p.ownerUID != "" {
		// Not a controller: the Deployment does not manage these Pods, it
		// only takes them along when it is deleted.
		meta["ownerReferences"] = []any{map[string]any{
			"apiVersion": "apps/v1", "kind": "Deployment", "name": p.cfg.OwnerDeployment, "uid": p.ownerUID,
		}}
	}
	return map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": meta, "spec": spec}
}

// traced wraps the transport of the Kubernetes API client so that requests
// which change something (create or delete a sandbox Pod) are client spans in
// the trace of their context. A GET is not traced: the Pod is looked up for
// every command that runs in a sandbox and polled while it starts, which
// would bury every trace. Client metrics are not recorded.
func traced(base http.RoundTripper, opts ...otelhttp.Option) http.RoundTripper {
	return otelhttp.NewTransport(base, append([]otelhttp.Option{
		otelhttp.WithFilter(func(r *http.Request) bool { return r.Method != http.MethodGet }),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string { return r.Method + " " + route(r.URL.Path) }),
		otelhttp.WithMeterProvider(noop.NewMeterProvider()),
	}, opts...)...)
}

// route is the path of a Kubernetes API request without the names in it:
// /api/v1/namespaces/{namespace}/pods/{name}. A span is named after it.
func route(path string) string {
	seg := strings.Split(path, "/")
	for i := 1; i < len(seg); i++ {
		switch seg[i-1] {
		case "namespaces":
			seg[i] = "{namespace}"
		case "pods", "deployments":
			seg[i] = "{name}"
		}
	}
	return strings.Join(seg, "/")
}
