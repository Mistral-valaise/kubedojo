package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/codes"

	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
	"github.com/Mistral-valaise/kubedojo/internal/telemetry/teltest"
)

// fakeTools puts a `docker` and a `kind` in front of the PATH that run no
// containers: they log how they were called and succeed, except for a call
// whose arguments start with failOn. It returns the file the calls go to.
func fakeTools(t *testing.T, failOn string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake docker and kind are shell scripts")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := `#!/bin/sh
tool=$(basename "$0")
echo "$tool $*" >> "$KD_FAKE_LOG"
case "$tool $*" in
  "$KD_FAKE_FAIL"*) echo "ERROR: the fake $tool was told to fail" >&2; exit 1 ;;
  "kind get nodes"*) echo "cka1234-control-plane"; echo "cka1234-worker" ;;
  "docker inspect"*) echo true ;;
esac
exit 0
`
	for _, name := range []string{"docker", "kind"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KD_FAKE_LOG", log)
	if failOn == "" {
		failOn = "nothing fails"
	}
	t.Setenv("KD_FAKE_FAIL", failOn)
	return log
}

func silent(string, ...any) {}

// Creating a cluster is four spans below the caller's, one per phase, however
// many docker commands a phase takes.
func TestCreateClusterIsOneSpanPerPhase(t *testing.T) {
	calls := fakeTools(t, "")
	k := &Kind{NodeImage: "kubedojo/node:v1.37.0", RegistryCache: true}
	rec := teltest.New()
	ctx, parent := rec.Start(context.Background(), "sandbox.env.create")
	err := k.CreateEnv(ctx, dockerhost.Local(), "abcd2345", "cka1234", EnvSpec{Name: "base", Workers: 1}, KeyPair{Private: "priv", Public: "pub"}, silent)
	parent.End(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("create a cluster:\n%s", teltest.Tree(rec.Ended()))
	want := "sandbox.env.create\n   kind.create_cluster\n   sandbox.nodes.init\n   sandbox.registry.configure\n   sandbox.nodes.wait_ready\n"
	if got := teltest.Shape(rec.Ended()); got != want {
		t.Fatalf("the trace has the shape\n%s\nwant\n%s", got, want)
	}
	// Eight commands ran; they are four spans. (A logged command can span
	// several lines: the script of a docker exec is one of its arguments.)
	raw, _ := os.ReadFile(calls)
	if n := strings.Count("\n"+string(raw), "\ndocker ") + strings.Count("\n"+string(raw), "\nkind "); n != 8 {
		t.Errorf("%d commands ran, want 8:\n%s", n, raw)
	}

	env := teltest.Attrs(rec.Named("sandbox.env.create")[0])
	if env["container.image.name"].AsString() != "kubedojo/node:v1.37.0" || env["kubedojo.kubernetes.version"].AsString() != "v1.37.0" ||
		env["kubedojo.environment.nodes"].AsInt64() != 2 {
		t.Errorf("attributes the driver adds to the caller's span: %v", env)
	}
	create := teltest.Attrs(rec.Named("kind.create_cluster")[0])
	if create["kubedojo.environment.alias"].AsString() != "cka1234" || create["container.image.name"].AsString() != "kubedojo/node:v1.37.0" {
		t.Errorf("kind.create_cluster: %v", create)
	}
	init := rec.Named("sandbox.nodes.init")[0]
	var nodes []string
	for _, e := range teltest.Events(init, "node.initialized") {
		nodes = append(nodes, e["node"].AsString())
	}
	if strings.Join(nodes, ",") != "cka1234-control-plane,cka1234-worker" {
		t.Errorf("events of sandbox.nodes.init: %v", nodes)
	}
	if m := teltest.Attrs(rec.Named("sandbox.registry.configure")[0])["kubedojo.registry.mirror"].AsString(); m != "http://kubedojo-registry-cache:5000" {
		t.Errorf("kubedojo.registry.mirror = %q", m)
	}
	// The session's key is on no span.
	for _, s := range rec.Ended() {
		for _, a := range s.Attributes() {
			if strings.Contains(a.Value.String(), "priv") {
				t.Errorf("%s: attribute %s carries the private key", s.Name(), a.Key)
			}
		}
	}
}

// Without an image cache there is no phase for it, and a phase that fails
// ends the creation: the spans say how far it got.
func TestCreateClusterStopsAtThePhaseThatFails(t *testing.T) {
	fakeTools(t, "docker exec cka1234-control-plane kubectl")
	k := &Kind{NodeImage: "registry.example:5000/node"}
	rec := teltest.New()
	ctx, parent := rec.Start(context.Background(), "sandbox.env.create")
	err := k.CreateEnv(ctx, dockerhost.Local(), "abcd2345", "cka1234", EnvSpec{Name: "base"}, KeyPair{}, silent)
	parent.End(err)
	if err == nil {
		t.Fatal("CreateEnv succeeded although the nodes never became Ready")
	}
	want := "sandbox.env.create\n   kind.create_cluster\n   sandbox.nodes.init\n   sandbox.nodes.wait_ready\n"
	if got := teltest.Shape(rec.Ended()); got != want {
		t.Fatalf("the trace has the shape\n%s\nwant\n%s", got, want)
	}
	if s := rec.Named("sandbox.nodes.wait_ready")[0]; s.Status().Code != codes.Error || !strings.Contains(s.Status().Description, "told to fail") {
		t.Errorf("sandbox.nodes.wait_ready: status %v %q", s.Status().Code, s.Status().Description)
	}
	if s := rec.Named("sandbox.nodes.init")[0]; s.Status().Code != codes.Unset {
		t.Errorf("sandbox.nodes.init: status %v", s.Status().Code)
	}
	// An image without a version tag names no Kubernetes version.
	if v, ok := teltest.Attrs(rec.Named("sandbox.env.create")[0])["kubedojo.kubernetes.version"]; ok {
		t.Errorf("kubedojo.kubernetes.version = %q for an image without a version", v.AsString())
	}
}

// A Linux host is two spans: starting the container, and one wait for sshd.
func TestCreateContainerIsTwoSpans(t *testing.T) {
	fakeTools(t, "")
	k := &Kind{LinuxImage: "kubedojo/linux:trixie"}
	rec := teltest.New()
	ctx, parent := rec.Start(context.Background(), "sandbox.env.create")
	err := k.CreateEnv(ctx, dockerhost.Local(), "abcd2345", "linux4821", EnvSpec{Name: "base", Type: EnvContainer}, KeyPair{Public: "pub"}, silent)
	parent.End(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("create a Linux host:\n%s", teltest.Tree(rec.Ended()))
	want := "sandbox.env.create\n   sandbox.container.start\n   sandbox.container.wait_ssh\n"
	if got := teltest.Shape(rec.Ended()); got != want {
		t.Fatalf("the trace has the shape\n%s\nwant\n%s", got, want)
	}
	if n := teltest.Attrs(rec.Named("sandbox.container.wait_ssh")[0])["polls"].AsInt64(); n != 1 {
		t.Errorf("polls = %d, want 1", n)
	}
	if img := teltest.Attrs(rec.Named("sandbox.env.create")[0])["container.image.name"].AsString(); img != "kubedojo/linux:trixie" {
		t.Errorf("container.image.name = %q", img)
	}
}

func TestImageVersion(t *testing.T) {
	for in, want := range map[string]string{
		"kubedojo/node:v1.37.0":            "v1.37.0",
		"ghcr.io/x/kubedojo-node:v1.36.2":  "v1.36.2",
		"kubedojo/node":                    "",
		"kubedojo/node:latest":             "",
		"registry.example:5000/node":       "",
		"registry.example:5000/node:v1.37": "v1.37",
	} {
		if got := imageVersion(in); got != want {
			t.Errorf("imageVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRedactURL(t *testing.T) {
	if got := redactURL("http://user:secret@mirror.example:5000"); strings.Contains(got, "secret") {
		t.Errorf("redactURL kept the password: %s", got)
	}
	if got := redactURL("http://registry-cache.kubedojo.svc:5000"); got != "http://registry-cache.kubedojo.svc:5000" {
		t.Errorf("redactURL changed a URL without a password: %s", got)
	}
}
