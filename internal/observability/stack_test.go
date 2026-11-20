package observability

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// docker is a Docker with at most one container and one volume. It answers
// the commands the stack sends and records them.
type docker struct {
	t       *testing.T
	calls   [][]string
	exists  bool
	running bool
	labels  map[string]string
	image   string
	volume  bool
	ready   bool
	// runOutput, when set, makes `docker run` fail with this output.
	runOutput string
	// publisher is the container `docker ps --filter publish=` reports.
	publisher string
	// ports is the answer of `docker port`.
	ports string
}

func (d *docker) run(_ context.Context, name string, args ...string) (string, error) {
	d.calls = append(d.calls, append([]string{name}, args...))
	if name != "docker" {
		d.t.Fatalf("unexpected command %s", name)
	}
	fail := errors.New("exit status 1")
	switch args[0] {
	case "inspect":
		if !d.exists {
			return "Error response from daemon: No such container: " + Container, fail
		}
		return fmt.Sprintf("%v|%s|%s|%s", d.running, d.labels[componentLabel], d.labels[hashLabel], d.image), nil
	case "stop":
		d.running = false
		return Container, nil
	case "rm":
		d.exists, d.running = false, false
		return Container, nil
	case "run":
		if d.runOutput != "" {
			return d.runOutput, fail
		}
		d.exists, d.running, d.labels = true, true, map[string]string{}
		for i, a := range args {
			if a == "--label" {
				k, v, _ := strings.Cut(args[i+1], "=")
				d.labels[k] = v
			}
			if a == "-v" && strings.HasPrefix(args[i+1], Volume+":") {
				d.volume = true
			}
		}
		d.image = args[len(args)-1]
		return "0123456789ab", nil
	case "exec":
		if d.ready {
			return "", nil
		}
		return "", fail
	case "logs":
		return "Error: Loki exited before the stack became ready.", nil
	case "ps":
		return d.publisher, nil
	case "port":
		return d.ports, nil
	case "volume":
		if args[1] == "rm" {
			if !d.volume {
				return "Error response from daemon: get " + Volume + ": no such volume", fail
			}
			d.volume = false
			return Volume, nil
		}
		if !d.volume {
			return "no such volume", fail
		}
		return "[]", nil
	}
	d.t.Fatalf("unexpected docker %v", args)
	return "", nil
}

// ran returns the calls of one docker subcommand.
func (d *docker) ran(sub string) [][]string {
	var out [][]string
	for _, c := range d.calls {
		if c[1] == sub {
			out = append(out, c)
		}
	}
	return out
}

func newStack(t *testing.T, d *docker, opt Options) *Stack {
	t.Helper()
	d.t = t
	if opt.DataDir == "" {
		opt.DataDir = t.TempDir()
	}
	s, err := New(opt, d.run)
	if err != nil {
		t.Fatal(err)
	}
	s.portFree = func(int) error { return nil }
	s.healthy = func(context.Context, string) error { return nil }
	s.poll = time.Millisecond
	s.ReadyTimeout = 200 * time.Millisecond
	return s
}

func TestUpStartsAPinnedLoopbackStack(t *testing.T) {
	d := &docker{ready: true}
	s := newStack(t, d, Options{Version: "v9"})
	if err := s.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	runs := d.ran("run")
	if len(runs) != 1 {
		t.Fatalf("docker run was called %d times", len(runs))
	}
	args := runs[0]
	line := strings.Join(args, " ")
	for _, want := range []string{
		"--name " + Container,
		"--label kubedojo.component=observability",
		"--label kubedojo.version=v9",
		"-p 127.0.0.1:3000:3000",
		"-p 127.0.0.1:4318:4318",
		"-v " + Volume + ":/data",
		"--storage.tsdb.retention.time=168h",
		"-store.retention=168h",
		"block-retention=168h",
		":/otel-lgtm/kubedojo-dashboards:ro",
		":/otel-lgtm/grafana/conf/provisioning/dashboards/kubedojo.yaml:ro",
		":/otel-lgtm/grafana/conf/provisioning/datasources/grafana-datasources.yaml:ro",
		":/otel-lgtm/grafana/conf/provisioning/alerting/kubedojo.yaml:ro",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("docker run lacks %q:\n%s", want, line)
		}
	}
	// Nothing listens beyond this machine.
	for i, a := range args {
		if a == "-p" && !strings.HasPrefix(args[i+1], "127.0.0.1:") {
			t.Errorf("port %s is published beyond loopback", args[i+1])
		}
	}
	// The image is pinned: a tag and a digest, never latest.
	image := args[len(args)-1]
	if image != Image || !strings.Contains(image, "@sha256:") || strings.Contains(image, "latest") {
		t.Errorf("image %q is not the pinned one", image)
	}
	// The dashboards are on disk where they are mounted from.
	dash, _ := filepath.Glob(filepath.Join(s.dir(), "dashboards", "*.json"))
	if len(dash) < 3 {
		t.Errorf("dashboards on disk: %v", dash)
	}
	if s.Endpoint() != "http://127.0.0.1:4318" || s.GrafanaURL() != "http://127.0.0.1:3000" {
		t.Errorf("endpoint %s, Grafana %s", s.Endpoint(), s.GrafanaURL())
	}
}

func TestUpKeepsARunningStackWithTheSameSettings(t *testing.T) {
	d := &docker{ready: true}
	s := newStack(t, d, Options{})
	for range 3 {
		if err := s.Up(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(d.ran("run")); n != 1 {
		t.Fatalf("docker run was called %d times for three starts", n)
	}
	if n := len(d.ran("rm")); n != 0 {
		t.Fatalf("a running stack was removed %d times", n)
	}
	st, err := s.Status(context.Background())
	if err != nil || !st.Running || !st.Owned || !st.Volume || !s.Current(st) {
		t.Fatalf("status %+v, err %v", st, err)
	}
}

func TestUpReplacesAStackWithOtherSettings(t *testing.T) {
	for name, change := range map[string]func(*Options){
		"port":      func(o *Options) { o.GrafanaPort = 3001 },
		"retention": func(o *Options) { o.Retention = 48 * time.Hour },
		"ephemeral": func(o *Options) { o.Ephemeral = true },
		"image":     func(o *Options) { o.Image = "grafana/otel-lgtm:0.99.0" },
	} {
		t.Run(name, func(t *testing.T) {
			d := &docker{ready: true}
			opt := Options{DataDir: t.TempDir()}
			if err := newStack(t, d, opt).Up(context.Background()); err != nil {
				t.Fatal(err)
			}
			change(&opt)
			next := newStack(t, d, opt)
			if st, _ := next.Status(context.Background()); next.Current(st) {
				t.Fatal("the running stack counts as current although the settings changed")
			}
			if err := next.Up(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(d.ran("rm")) != 1 || len(d.ran("run")) != 2 {
				t.Fatalf("rm %d, run %d; want the stack replaced once", len(d.ran("rm")), len(d.ran("run")))
			}
			if name == "ephemeral" && slices.Contains(d.ran("run")[1], Volume+":/data") {
				t.Fatal("an ephemeral stack mounts the data volume")
			}
		})
	}
}

func TestUpRestartsAStoppedStack(t *testing.T) {
	d := &docker{ready: true}
	s := newStack(t, d, Options{})
	if err := s.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	d.running = false
	if err := s.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(d.ran("rm")) != 1 || len(d.ran("run")) != 2 || !d.running {
		t.Fatalf("rm %d, run %d, running %v", len(d.ran("rm")), len(d.ran("run")), d.running)
	}
}

// Releases up to 0.4.0 started the container without labels. It is ours and
// is replaced; any other container with the name is somebody else's.
func TestForeignContainersAreNeverTouched(t *testing.T) {
	legacy := &docker{ready: true, exists: true, running: true, labels: map[string]string{}, image: "grafana/otel-lgtm:latest"}
	if err := newStack(t, legacy, Options{}).Up(context.Background()); err != nil {
		t.Fatalf("a container of an older kubedojo was not taken over: %v", err)
	}
	if len(legacy.ran("rm")) != 1 || legacy.labels[componentLabel] != component {
		t.Fatalf("the old container was not replaced by a labelled one: %v", legacy.labels)
	}

	foreign := &docker{exists: true, running: true, labels: map[string]string{}, image: "postgres:17"}
	s := newStack(t, foreign, Options{})
	if err := s.Up(context.Background()); err == nil || !strings.Contains(err.Error(), "did not create") {
		t.Fatalf("Up with a foreign container = %v", err)
	}
	if _, err := s.Stop(context.Background()); err == nil {
		t.Fatal("Stop removed a foreign container")
	}
	if err := s.Reset(context.Background()); err == nil {
		t.Fatal("Reset removed a foreign container")
	}
	if n := len(foreign.ran("rm")) + len(foreign.ran("run")); n != 0 {
		t.Fatalf("a foreign container was touched: %v", foreign.calls)
	}
}

func TestOccupiedPortNamesThePortTheFlagAndTheOwner(t *testing.T) {
	d := &docker{ready: true, publisher: "my-grafana"}
	s := newStack(t, d, Options{})
	s.portFree = func(p int) error {
		if p == 3000 {
			return errors.New("address already in use")
		}
		return nil
	}
	err := s.Up(context.Background())
	if err == nil {
		t.Fatal("Up succeeded although port 3000 is taken")
	}
	for _, want := range []string{"3000", "Grafana", "--grafana-port", "my-grafana"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
	if len(d.ran("run")) != 0 {
		t.Fatal("docker run was attempted with an occupied port")
	}

	// Docker can still lose the race for the port.
	d = &docker{runOutput: "docker: Error response from daemon: Bind for 127.0.0.1:4318 failed: port is already allocated"}
	err = newStack(t, d, Options{}).Up(context.Background())
	if err == nil || !strings.Contains(err.Error(), "--otlp-port") {
		t.Fatalf("Up = %v, want a hint at the port flags", err)
	}
	if len(d.ran("rm")) != 1 {
		t.Fatal("the container that failed to start was left behind")
	}
}

// A restart with other settings must not take the running stack down when the
// new one cannot start. The ports the running stack holds itself do not count
// as occupied.
func TestAFailedRestartKeepsTheRunningStack(t *testing.T) {
	d := &docker{ready: true, ports: "3000/tcp -> 127.0.0.1:3000\n4318/tcp -> 127.0.0.1:4318"}
	dir := t.TempDir()
	if err := newStack(t, d, Options{DataDir: dir}).Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	taken := func(p int) error { return fmt.Errorf("port %d: address already in use", p) }

	// Another OTLP port, which something else holds.
	s := newStack(t, d, Options{DataDir: dir, OTLPPort: 4319})
	s.portFree = taken
	if err := s.Up(context.Background()); err == nil || !strings.Contains(err.Error(), "4319") {
		t.Fatalf("Up = %v, want the occupied port 4319", err)
	}
	if !d.running || len(d.ran("rm")) != 0 {
		t.Fatal("the running stack was removed although its replacement cannot start")
	}

	// Another retention on the same ports: both are held by the stack itself.
	s = newStack(t, d, Options{DataDir: dir, Retention: 48 * time.Hour})
	s.portFree = taken
	if err := s.Up(context.Background()); err != nil {
		t.Fatalf("Up with the stack's own ports: %v", err)
	}
	if len(d.ran("rm")) != 1 || len(d.ran("run")) != 2 {
		t.Fatalf("rm %d, run %d; want the stack replaced once", len(d.ran("rm")), len(d.ran("run")))
	}
}

func TestUpReportsAStackThatNeverGetsReady(t *testing.T) {
	d := &docker{ready: false}
	s := newStack(t, d, Options{})
	start := time.Now()
	err := s.Up(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not ready after") || !strings.Contains(err.Error(), "Loki exited") {
		t.Fatalf("Up = %v, want a timeout that shows the container's log", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("the timeout of %s took %s", s.ReadyTimeout, time.Since(start))
	}

	// Grafana must answer too, not only the ready file exist.
	d = &docker{ready: true}
	s = newStack(t, d, Options{})
	s.healthy = func(context.Context, string) error { return errors.New("connection refused") }
	if err := s.Up(context.Background()); err == nil {
		t.Fatal("Up succeeded although Grafana does not answer")
	}
}

// A container that exits while starting is reported at once, not after the
// timeout: Loki, for one, exits on a flag it does not know.
func TestUpNoticesAStackThatExits(t *testing.T) {
	d := &docker{}
	s := newStack(t, d, Options{})
	s.ReadyTimeout = time.Minute
	s.run = func(ctx context.Context, name string, args ...string) (string, error) {
		o, err := d.run(ctx, name, args...)
		if args[0] == "run" {
			d.running = false
		}
		return o, err
	}
	start := time.Now()
	err := s.Up(context.Background())
	if err == nil || !strings.Contains(err.Error(), "stopped while starting") {
		t.Fatalf("Up = %v, want it to notice the exit", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("Up waited for the timeout although the container was gone")
	}
}

func TestStopKeepsTheDataAndResetRemovesIt(t *testing.T) {
	d := &docker{ready: true}
	dir := t.TempDir()
	// Learner progress lives next to the observability directory.
	db := filepath.Join(dir, "kubedojo.db")
	if err := os.WriteFile(db, []byte("progress"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newStack(t, d, Options{DataDir: dir})
	if err := s.Up(context.Background()); err != nil {
		t.Fatal(err)
	}

	removed, err := s.Stop(context.Background())
	if err != nil || !removed {
		t.Fatalf("Stop = %v, %v", removed, err)
	}
	if d.exists || !d.volume {
		t.Fatalf("after Stop: container exists=%v, volume=%v; want the volume kept", d.exists, d.volume)
	}
	if removed, err := s.Stop(context.Background()); err != nil || removed {
		t.Fatalf("second Stop = %v, %v", removed, err)
	}

	if err := s.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Reset(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d.exists || d.volume {
		t.Fatalf("after Reset: container exists=%v, volume=%v", d.exists, d.volume)
	}
	if _, err := os.Stat(s.dir()); !os.IsNotExist(err) {
		t.Fatalf("the observability directory survived the reset: %v", err)
	}
	if b, err := os.ReadFile(db); err != nil || string(b) != "progress" {
		t.Fatalf("the reset touched the learner's database: %q, %v", b, err)
	}
	// Resetting what is not there is not an error.
	if err := s.Reset(context.Background()); err != nil {
		t.Fatalf("second Reset: %v", err)
	}
}

func TestOptionsAreValidated(t *testing.T) {
	for name, opt := range map[string]Options{
		"same ports":      {DataDir: "x", GrafanaPort: 3000, OTLPPort: 3000},
		"port range":      {DataDir: "x", GrafanaPort: 70000},
		"short retention": {DataDir: "x", Retention: time.Hour},
		"no data dir":     {},
	} {
		if _, err := New(opt, nil); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCheckEndpoints(t *testing.T) {
	const local = "http://127.0.0.1:4318"
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	for name, c := range map[string]struct {
		env  map[string]string
		fail string // the variable the error must name; empty = no error
	}{
		"nothing set":          {nil, ""},
		"the local stack":      {map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:4318"}, ""},
		"localhost":            {map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4318/"}, ""},
		"local signal path":    {map[string]string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "http://127.0.0.1:4318/v1/traces"}, ""},
		"another collector":    {map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "https://otlp.example.com"}, "OTEL_EXPORTER_OTLP_ENDPOINT"},
		"another local port":   {map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:4319"}, "OTEL_EXPORTER_OTLP_ENDPOINT"},
		"one signal elsewhere": {map[string]string{"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT": "https://logs.example.com/v1/logs"}, "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"},
	} {
		err := CheckEndpoints(env(c.env), local)
		switch {
		case c.fail == "" && err != nil:
			t.Errorf("%s: %v", name, err)
		case c.fail != "" && (err == nil || !strings.Contains(err.Error(), c.fail) || !strings.Contains(err.Error(), "--observability")):
			t.Errorf("%s: error %v does not name %s and the flag", name, err, c.fail)
		}
	}
}

// The chart's evaluation stack mounts the same provisioning files, so it must
// run the image they were written for.
func TestChartRunsTheSameImage(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/helm/kubedojo/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		LGTM struct {
			Image string `yaml:"image"`
		} `yaml:"lgtm"`
	}
	if err := yaml.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if v.LGTM.Image != Image {
		t.Fatalf("values.yaml runs %q, kubedojo up --observability runs %q", v.LGTM.Image, Image)
	}
}

// The chart's evaluation stack is the same container. The flags it is started
// with are written a second time, in a Helm template; this keeps them equal.
func TestChartStartsTheStackWithTheSameFlags(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/helm/kubedojo/templates/lgtm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// The chart writes the retention as a template value.
	chart := strings.ReplaceAll(string(raw), "{{ $keep }}", "168h")
	s := newStack(t, &docker{}, Options{})
	_, prov, err := s.files()
	if err != nil {
		t.Fatal(err)
	}
	args, _ := s.args(prov)
	n := 0
	for i, a := range args {
		if a != "-e" {
			continue
		}
		name, value, _ := strings.Cut(args[i+1], "=")
		if !strings.HasSuffix(name, "_EXTRA_ARGS") {
			continue
		}
		n++
		if !strings.Contains(chart, "- name: "+name) || !strings.Contains(chart, `value: "`+value+`"`) {
			t.Errorf("templates/lgtm.yaml does not set %s to\n  %s", name, value)
		}
	}
	if n != 3 {
		t.Fatalf("%d of the three *_EXTRA_ARGS were compared", n)
	}
}
