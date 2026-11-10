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
	"os"
	"strings"
	"time"

	"github.com/kubedojo/kubedojo/internal/dockerhost"
)

const saDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// Config describes the sandbox Pods.
type Config struct {
	Namespace    string            // where sandbox Pods are created
	Image        string            // Docker-in-Docker image with the sandbox images preloaded
	CPURequest   string            // e.g. "1"
	MemRequest   string            // e.g. "3Gi"
	MemLimit     string            // e.g. "6Gi"
	NodeSelector map[string]string // dedicated node pool
	Toleration   string            // taint key tolerated by sandbox Pods (effect NoSchedule)
	PullSecret   string
}

type Provider struct {
	cfg    Config
	api    string
	client *http.Client
}

// New builds a provider from the in-cluster ServiceAccount.
func New(cfg Config) (*Provider, error) {
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
	return &Provider{cfg: cfg, api: "https://" + net.JoinHostPort(host, port),
		client: &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}}, nil
}

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
	token, err := os.ReadFile(saDir + "/token") // re-read: projected tokens rotate
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
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
	Status struct {
		Phase      string `json:"phase"`
		PodIP      string `json:"podIP"`
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions"`
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

// Acquire returns the Docker endpoint of the session's sandbox Pod, creating
// the Pod on first use.
func (p *Provider) Acquire(ctx context.Context, sessionID string, logf func(string, ...any)) (dockerhost.Host, error) {
	name := podName(sessionID)
	st, code, err := p.get(ctx, name)
	if err != nil {
		return dockerhost.Host{}, err
	}
	if code == 200 && ready(st) {
		return dockerhost.Host{Endpoint: "tcp://" + net.JoinHostPort(st.Status.PodIP, "2375")}, nil
	}
	if code == 404 {
		logf("Starting the sandbox host ...")
		c, body, err := p.do(ctx, "POST", "/api/v1/namespaces/"+p.cfg.Namespace+"/pods", p.manifest(name, sessionID))
		if err != nil {
			return dockerhost.Host{}, err
		}
		if c != 201 && c != 409 {
			return dockerhost.Host{}, fmt.Errorf("create sandbox pod: HTTP %d: %s", c, strings.TrimSpace(string(body)))
		}
	} else if code != 200 {
		return dockerhost.Host{}, fmt.Errorf("get sandbox pod: HTTP %d", code)
	}
	deadline := time.Now().Add(8 * time.Minute)
	for {
		st, code, err := p.get(ctx, name)
		if err != nil {
			return dockerhost.Host{}, err
		}
		if code == 200 && ready(st) {
			logf("Sandbox host is ready.")
			return dockerhost.Host{Endpoint: "tcp://" + net.JoinHostPort(st.Status.PodIP, "2375")}, nil
		}
		if code == 200 && st.Status.Phase == "Failed" {
			return dockerhost.Host{}, fmt.Errorf("sandbox pod %s failed", name)
		}
		if time.Now().After(deadline) {
			return dockerhost.Host{}, fmt.Errorf("sandbox pod %s did not become ready in time", name)
		}
		select {
		case <-ctx.Done():
			return dockerhost.Host{}, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// Release deletes the session's sandbox Pod; everything inside goes with it.
func (p *Provider) Release(ctx context.Context, sessionID string) error {
	code, body, err := p.do(ctx, "DELETE", "/api/v1/namespaces/"+p.cfg.Namespace+"/pods/"+podName(sessionID)+"?gracePeriodSeconds=5", nil)
	if err != nil {
		return err
	}
	if code != 200 && code != 202 && code != 404 {
		return fmt.Errorf("delete sandbox pod: HTTP %d: %s", code, strings.TrimSpace(string(body)))
	}
	return nil
}

func (p *Provider) manifest(name, sessionID string) map[string]any {
	res := map[string]any{"requests": map[string]string{"cpu": p.cfg.CPURequest, "memory": p.cfg.MemRequest}}
	if p.cfg.MemLimit != "" {
		res["limits"] = map[string]string{"memory": p.cfg.MemLimit}
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
			map[string]any{"name": "docker", "emptyDir": map[string]any{}},
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
	return map[string]any{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]any{
			"name": name,
			"labels": map[string]string{
				"app.kubernetes.io/name": "kubedojo", "app.kubernetes.io/component": "sandbox",
				"kubedojo.dev/session": sessionID,
			},
		},
		"spec": spec,
	}
}
