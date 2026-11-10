// Package dockerhost abstracts *where* sandboxes run. The sandbox code only
// ever talks to "a Docker endpoint": locally that is the machine's Docker
// socket, in a hosted deployment it is the Docker-in-Docker daemon inside a
// per-session pod. Everything above this package is identical in both modes.
package dockerhost

import (
	"context"
	"os"
	"os/exec"

	"github.com/moby/moby/client"
)

// Host is one Docker endpoint.
type Host struct {
	// Endpoint is a DOCKER_HOST value (e.g. tcp://10.1.2.3:2376). Empty means
	// "whatever the local environment / docker context resolves to".
	Endpoint string
	// CertPath enables TLS verification against the endpoint when set.
	CertPath string
}

// Local returns the Docker endpoint of the machine kubedojo runs on.
func Local() Host { return Host{} }

// Env returns the process environment for CLI tools (docker, kind).
func (h Host) Env() []string {
	env := os.Environ()
	if h.Endpoint != "" {
		env = append(env, "DOCKER_HOST="+h.Endpoint)
	}
	if h.CertPath != "" {
		env = append(env, "DOCKER_TLS_VERIFY=1", "DOCKER_CERT_PATH="+h.CertPath)
	}
	return env
}

// Command builds an exec.Cmd bound to this endpoint.
func (h Host) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = h.Env()
	return cmd
}

// Client returns an Engine API client for this endpoint (used for PTY attach).
func (h Host) Client() (*client.Client, error) {
	var opts []client.Opt
	switch {
	case h.Endpoint == "":
		if os.Getenv("DOCKER_HOST") != "" {
			opts = append(opts, client.FromEnv)
		} else if sock := localSocket(); sock != "" {
			opts = append(opts, client.WithHost(sock))
		}
	default:
		opts = append(opts, client.WithHost(h.Endpoint))
		if h.CertPath != "" {
			opts = append(opts, client.WithTLSClientConfig(
				h.CertPath+"/ca.pem", h.CertPath+"/cert.pem", h.CertPath+"/key.pem"))
		}
	}
	return client.New(opts...)
}

// localSocket asks the docker CLI which endpoint the active context uses, so
// Docker Desktop, OrbStack, Colima and Rancher Desktop all work unconfigured.
func localSocket() string {
	out, err := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output()
	if err != nil {
		return ""
	}
	s := string(out)
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
