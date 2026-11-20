// Package observability runs the local observability stack of
// `kubedojo up --observability`: Grafana's all-in-one LGTM container with
// Grafana, Prometheus, Loki, Tempo and an OpenTelemetry Collector.
//
// The stack is one container and, unless it is ephemeral, one volume. Both are
// owned by kubedojo: they carry its name and label, and nothing else is ever
// stopped or removed.
package observability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Mistral-valaise/kubedojo/deploy/helm/kubedojo/dashboards"
	provisioning "github.com/Mistral-valaise/kubedojo/deploy/helm/kubedojo/observability"
)

const (
	// Image is the tested version of grafana/otel-lgtm, pinned by digest: the
	// provisioning paths and the flags below are those of this version.
	Image = "grafana/otel-lgtm:0.41.1@sha256:1b852b77d7bcadb7699bfe7c78a51d059dff85168de674728cf7abf4474645e5"

	Container = "kubedojo-lgtm"
	Volume    = "kubedojo-lgtm-data"

	DefaultGrafanaPort = 3000
	DefaultOTLPPort    = 4318
	// DefaultRetention bounds what a laptop keeps: with a few sessions a day
	// a week of metrics, logs and traces stays well below a gigabyte.
	DefaultRetention = 7 * 24 * time.Hour
	// MinRetention is the shortest period Loki accepts.
	MinRetention = 24 * time.Hour

	componentLabel = "kubedojo.component"
	component      = "observability"
	hashLabel      = "kubedojo.config-hash"
	versionLabel   = "kubedojo.version"
)

// Runner runs a command and returns its combined output.
type Runner func(ctx context.Context, name string, args ...string) (string, error)

// Options describe the stack. The zero value of a field selects its default.
type Options struct {
	// DataDir is kubedojo's data directory. The dashboards and provisioning
	// files are written to its observability subdirectory and mounted from there.
	DataDir     string
	Image       string
	GrafanaPort int
	OTLPPort    int
	// Ephemeral keeps metrics, logs and traces inside the container: they are
	// gone when it is recreated.
	Ephemeral bool
	Retention time.Duration
	// Version is the kubedojo version, recorded on the container.
	Version string
	// Name is the name of the container; its volume is Name-data. The default
	// is Container. Tests run a stack of their own next to the user's.
	Name string
}

func (o Options) withDefaults() Options {
	if o.Image == "" {
		o.Image = Image
	}
	if o.GrafanaPort == 0 {
		o.GrafanaPort = DefaultGrafanaPort
	}
	if o.OTLPPort == 0 {
		o.OTLPPort = DefaultOTLPPort
	}
	if o.Retention == 0 {
		o.Retention = DefaultRetention
	}
	if o.Name == "" {
		o.Name = Container
	}
	return o
}

func (o Options) validate() error {
	for name, p := range map[string]int{"Grafana": o.GrafanaPort, "OTLP": o.OTLPPort} {
		if p < 1 || p > 65535 {
			return fmt.Errorf("%s port %d is not a port number", name, p)
		}
	}
	if o.GrafanaPort == o.OTLPPort {
		return fmt.Errorf("the Grafana and the OTLP port are both %d", o.GrafanaPort)
	}
	if o.Retention < MinRetention {
		return fmt.Errorf("retention %s is too short: Loki keeps at least %s", o.Retention, MinRetention)
	}
	if o.DataDir == "" {
		return errors.New("no data directory")
	}
	return nil
}

// Stack is the local observability stack.
type Stack struct {
	opt Options
	run Runner
	// Out receives progress lines for the person at the terminal.
	Out io.Writer
	// ReadyTimeout is how long Up waits for the stack to answer.
	ReadyTimeout time.Duration

	// Replaced in tests.
	portFree func(port int) error
	healthy  func(ctx context.Context, url string) error
	poll     time.Duration
}

func New(opt Options, run Runner) (*Stack, error) {
	opt = opt.withDefaults()
	if err := opt.validate(); err != nil {
		return nil, err
	}
	return &Stack{opt: opt, run: run, Out: io.Discard, ReadyTimeout: 2 * time.Minute,
		portFree: portFree, healthy: grafanaHealthy, poll: time.Second}, nil
}

// Endpoint is where kubedojo sends OTLP over HTTP.
func (s *Stack) Endpoint() string { return fmt.Sprintf("http://127.0.0.1:%d", s.opt.OTLPPort) }

// GrafanaURL is the address of Grafana.
func (s *Stack) GrafanaURL() string { return fmt.Sprintf("http://127.0.0.1:%d", s.opt.GrafanaPort) }

func (s *Stack) name() string   { return s.opt.Name }
func (s *Stack) volume() string { return s.opt.Name + "-data" }

func (s *Stack) dir() string { return filepath.Join(s.opt.DataDir, "observability") }

func (s *Stack) say(format string, a ...any) { fmt.Fprintf(s.Out, format+"\n", a...) }

// file is one file kubedojo writes and mounts read-only into the container.
type file struct {
	rel    string // below dir()
	target string // in the container
	data   []byte
}

// files returns what is mounted: the dashboards as one directory, and each
// provisioning file on its own.
func (s *Stack) files() (dash []file, prov []file, err error) {
	var names []string
	for _, pattern := range dashboards.Local {
		m, err := fs.Glob(dashboards.FS, pattern)
		if err != nil {
			return nil, nil, err
		}
		names = append(names, m...)
	}
	sort.Strings(names)
	for _, n := range names {
		b, err := dashboards.FS.ReadFile(n)
		if err != nil {
			return nil, nil, err
		}
		// One flat directory: Grafana's provider puts them all in one folder.
		dash = append(dash, file{rel: filepath.Join("dashboards", path.Base(n)), data: b})
	}
	prov = []file{
		{rel: "kubedojo.yaml", target: provisioning.DashboardProviderPath, data: provisioning.DashboardProvider},
		{rel: "datasources.yaml", target: provisioning.DatasourcesPath, data: provisioning.Datasources},
		{rel: "alerts.yaml", target: provisioning.GrafanaAlertsPath, data: provisioning.GrafanaAlerts},
	}
	return dash, prov, nil
}

// write puts the mounted files on disk. Dashboards that are no longer shipped
// are removed, so that Grafana drops them too.
func (s *Stack) write(dash, prov []file) error {
	dd := filepath.Join(s.dir(), "dashboards")
	if err := os.MkdirAll(dd, 0o755); err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, f := range append(append([]file(nil), dash...), prov...) {
		p := filepath.Join(s.dir(), f.rel)
		keep[p] = true
		if err := os.WriteFile(p, f.data, 0o644); err != nil {
			return err
		}
	}
	old, _ := filepath.Glob(filepath.Join(dd, "*.json"))
	for _, p := range old {
		if !keep[p] {
			_ = os.Remove(p)
		}
	}
	return nil
}

// args are the arguments of `docker run`. hash identifies everything that is
// fixed when the container is created: a running container with another hash
// was started with other settings and is replaced.
func (s *Stack) args(prov []file) (args []string, hash string) {
	o := s.opt
	hours := fmt.Sprintf("%dh", int(o.Retention.Hours()))
	args = []string{
		"-p", fmt.Sprintf("127.0.0.1:%d:3000", o.GrafanaPort),
		"-p", fmt.Sprintf("127.0.0.1:%d:4318", o.OTLPPort),
		"-v", filepath.Join(s.dir(), "dashboards") + ":" + provisioning.DashboardsPath + ":ro",
	}
	h := sha256.New()
	for _, f := range prov {
		args = append(args, "-v", filepath.Join(s.dir(), f.rel)+":"+f.target+":ro")
		// Grafana reads provisioning files once, at start.
		h.Write(f.data)
	}
	if !o.Ephemeral {
		args = append(args, "-v", s.volume()+":/data")
	}
	args = append(args,
		"-e", "PROMETHEUS_EXTRA_ARGS=--storage.tsdb.retention.time="+hours,
		"-e", "LOKI_EXTRA_ARGS=-store.retention="+hours+" -compactor.retention-enabled=true -compactor.delete-request-store=filesystem -compactor.working-directory=/data/loki/compactor",
		// Tempo keeps the traces of the last minutes in its live store, which
		// the image leaves outside the volume: without the paths, a restart
		// loses every trace that was not written to a block yet.
		"-e", "TEMPO_EXTRA_ARGS=-backend-worker.compaction.block-retention="+hours+" -backend-scheduler.provider.work.compaction.block-retention="+hours+
			" -live-store.wal.path=/data/tempo/live-store/traces -live-store.shutdown_marker_dir=/data/tempo/live-store/shutdown-marker",
	)
	fmt.Fprintln(h, o.Image)
	fmt.Fprintln(h, strings.Join(args, "\x00"))
	hash = hex.EncodeToString(h.Sum(nil))[:16]
	head := []string{"run", "-d", "--name", s.name(),
		"--label", componentLabel + "=" + component,
		"--label", hashLabel + "=" + hash,
		"--label", versionLabel + "=" + o.Version}
	return append(append(head, args...), o.Image), hash
}

// State is what `docker inspect` says about the container.
type State struct {
	Exists  bool
	Running bool
	// Owned is false for a container with kubedojo's name that kubedojo did
	// not create. It is never touched.
	Owned bool
	Hash  string
	Image string
	// Volume reports whether the data volume exists.
	Volume bool
}

func (s *Stack) inspect(ctx context.Context) (State, error) {
	var st State
	const format = `{{.State.Running}}|{{index .Config.Labels "` + componentLabel + `"}}|{{index .Config.Labels "` + hashLabel + `"}}|{{.Config.Image}}`
	o, err := s.run(ctx, "docker", "inspect", "-f", format, s.name())
	if err != nil {
		if strings.Contains(strings.ToLower(o), "no such") {
			return st, nil
		}
		return st, dockerError("inspect "+s.name(), o, err)
	}
	p := strings.SplitN(strings.TrimSpace(o), "|", 4)
	if len(p) != 4 {
		return st, fmt.Errorf("unexpected answer from docker inspect: %q", o)
	}
	st.Exists, st.Running, st.Hash, st.Image = true, p[0] == "true", p[2], p[3]
	// Releases up to 0.4.0 created the container without labels.
	st.Owned = p[1] == component || (p[1] == "" && strings.HasPrefix(st.Image, "grafana/otel-lgtm"))
	return st, nil
}

// Status reports the state of the container and of the data volume.
func (s *Stack) Status(ctx context.Context) (State, error) {
	st, err := s.inspect(ctx)
	if err != nil {
		return st, err
	}
	_, verr := s.run(ctx, "docker", "volume", "inspect", s.volume())
	st.Volume = verr == nil
	return st, nil
}

// Current reports whether a running container was started with these options.
func (s *Stack) Current(st State) bool {
	_, prov, err := s.files()
	if err != nil {
		return false
	}
	_, hash := s.args(prov)
	return st.Running && st.Hash == hash
}

func (s *Stack) errForeign() error {
	return fmt.Errorf("a container named %s exists that kubedojo did not create; remove or rename it (docker rm -f %s)", s.name(), s.name())
}

// Up starts the stack, or keeps the one that is running with the same
// settings, and returns when Grafana and the collector answer.
func (s *Stack) Up(ctx context.Context) error {
	dash, prov, err := s.files()
	if err != nil {
		return err
	}
	if err := s.write(dash, prov); err != nil {
		return err
	}
	args, hash := s.args(prov)
	st, err := s.inspect(ctx)
	if err != nil {
		return err
	}
	if st.Exists && !st.Owned {
		return s.errForeign()
	}
	if st.Running && st.Hash == hash {
		s.say("  ✓ the observability stack is running")
		return s.wait(ctx)
	}
	// Before the running stack is removed: if the new one cannot start, the
	// old one keeps running.
	if err := s.portsFree(ctx, st.Running); err != nil {
		return err
	}
	if st.Exists {
		if st.Running {
			s.say("  … restarting the observability stack with the new settings")
		}
		if err := s.remove(ctx, st.Running); err != nil {
			return err
		}
	}
	s.say("  … starting the observability stack (%s)", strings.SplitN(s.opt.Image, "@", 2)[0])
	if o, err := s.run(ctx, "docker", args...); err != nil {
		// Docker reserves the port only now; another process may have won.
		if strings.Contains(o, "port is already allocated") || strings.Contains(o, "address already in use") {
			_, _ = s.run(ctx, "docker", "rm", "-f", s.name())
			return fmt.Errorf("a port of the observability stack is in use; choose others with --grafana-port and --otlp-port (docker says: %s)", lastLine(o))
		}
		return dockerError("start "+s.name(), o, err)
	}
	return s.wait(ctx)
}

// portsFree checks that the ports can be published. Ports the running stack
// holds itself are free as soon as it is replaced.
func (s *Stack) portsFree(ctx context.Context, running bool) error {
	own := map[string]bool{}
	if running {
		// 3000/tcp -> 127.0.0.1:3000
		o, _ := s.run(ctx, "docker", "port", s.name())
		for _, line := range strings.Split(o, "\n") {
			if i := strings.LastIndex(line, ":"); i >= 0 {
				own[strings.TrimSpace(line[i+1:])] = true
			}
		}
	}
	for _, p := range []struct {
		port       int
		what, flag string
	}{{s.opt.GrafanaPort, "Grafana", "--grafana-port"}, {s.opt.OTLPPort, "OTLP", "--otlp-port"}} {
		if own[fmt.Sprint(p.port)] {
			continue
		}
		if err := s.portFree(p.port); err != nil {
			by := ""
			if name, _ := s.run(ctx, "docker", "ps", "--filter", fmt.Sprintf("publish=%d", p.port), "--format", "{{.Names}}"); strings.TrimSpace(name) != "" {
				by = fmt.Sprintf(" by the container %q", strings.Fields(name)[0])
			}
			return fmt.Errorf("port %d (%s) is already in use%s; choose another with %s", p.port, p.what, by, p.flag)
		}
	}
	return nil
}

// wait returns when the container reports all components ready and Grafana
// answers on the published port.
func (s *Stack) wait(ctx context.Context) error {
	deadline := time.Now().Add(s.ReadyTimeout)
	announced := false
	for {
		st, err := s.inspect(ctx)
		if err != nil {
			return err
		}
		if !st.Running {
			return fmt.Errorf("the observability stack stopped while starting:\n%s", s.logs(ctx))
		}
		if _, err := s.run(ctx, "docker", "exec", s.name(), "test", "-f", "/tmp/ready"); err == nil {
			if err := s.healthy(ctx, s.GrafanaURL()+"/api/health"); err == nil {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the observability stack was not ready after %s:\n%s", s.ReadyTimeout, s.logs(ctx))
		}
		if !announced {
			s.say("  … waiting for Grafana, Prometheus, Loki, Tempo and the collector")
			announced = true
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.poll):
		}
	}
}

func (s *Stack) logs(ctx context.Context) string {
	o, _ := s.run(ctx, "docker", "logs", "--tail", "15", s.name())
	return strings.TrimSpace(o) + "\n(all of it: docker logs " + s.name() + ")"
}

// remove stops the container and removes it. A running stack is asked to
// stop first: killed, Tempo and Loki lose what they had not written to disk
// yet, which is the last minutes before the stop.
func (s *Stack) remove(ctx context.Context, running bool) error {
	if running {
		// The image gives its components five seconds by default.
		_, _ = s.run(ctx, "docker", "stop", "-t", "20", s.name())
	}
	if o, err := s.run(ctx, "docker", "rm", "-f", s.name()); err != nil {
		return dockerError("remove "+s.name(), o, err)
	}
	return nil
}

// Stop removes the container. The data volume stays, so the next start finds
// the metrics, logs and traces again. It reports whether there was a container.
func (s *Stack) Stop(ctx context.Context) (bool, error) {
	st, err := s.inspect(ctx)
	if err != nil || !st.Exists {
		return false, err
	}
	if !st.Owned {
		return false, s.errForeign()
	}
	if err := s.remove(ctx, st.Running); err != nil {
		return false, err
	}
	return true, nil
}

// Reset removes the container, the data volume and the files written to the
// data directory. Learner progress lives elsewhere and is not touched.
func (s *Stack) Reset(ctx context.Context) error {
	if _, err := s.Stop(ctx); err != nil {
		return err
	}
	if o, err := s.run(ctx, "docker", "volume", "rm", s.volume()); err != nil && !strings.Contains(strings.ToLower(o), "no such volume") {
		return dockerError("remove the volume "+s.volume(), o, err)
	}
	return os.RemoveAll(s.dir())
}

// CheckEndpoints refuses to start the local stack when telemetry is already
// configured to go elsewhere: kubedojo would otherwise have to pick one
// destination silently. getenv is os.Getenv.
func CheckEndpoints(getenv func(string) string, local string) error {
	for _, k := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"} {
		v := getenv(k)
		if v == "" || sameEndpoint(v, local) {
			continue
		}
		return fmt.Errorf("%s is set to %s, but --observability sends telemetry to the local stack at %s. "+
			"Unset %s to use the local stack, or drop --observability to keep your own destination", k, v, local, k)
	}
	return nil
}

// sameEndpoint compares the host and port of two OTLP endpoints; localhost
// and 127.0.0.1 are the same machine, and a signal endpoint carries a path.
func sameEndpoint(a, b string) bool {
	norm := func(s string) string {
		s = strings.TrimPrefix(strings.TrimPrefix(s, "http://"), "https://")
		s, _, _ = strings.Cut(s, "/")
		return strings.Replace(s, "localhost:", "127.0.0.1:", 1)
	}
	return norm(a) == norm(b)
}

func portFree(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	return ln.Close()
}

func grafanaHealthy(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func dockerError(what, out string, err error) error {
	if out = strings.TrimSpace(out); out != "" {
		return fmt.Errorf("%s: %s", what, lastLine(out))
	}
	return fmt.Errorf("%s: %w", what, err)
}
