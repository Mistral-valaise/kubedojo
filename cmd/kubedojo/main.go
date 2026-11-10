// kubedojo — hands-on Kubernetes exam training (CKA, CKS).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/kubedojo/kubedojo/content"
	"github.com/kubedojo/kubedojo/internal/api"
	"github.com/kubedojo/kubedojo/internal/catalog"
	"github.com/kubedojo/kubedojo/internal/sandbox"
	"github.com/kubedojo/kubedojo/internal/session"
	"github.com/kubedojo/kubedojo/internal/store"
	"github.com/kubedojo/kubedojo/internal/telemetry"
	"github.com/kubedojo/kubedojo/internal/webui"
)

var version = "dev"

const usage = `kubedojo — hands-on Kubernetes exam training (CKA, CKS)

Usage:
  kubedojo up        Start kubedojo locally and open it in your browser
  kubedojo serve     Run the server only (containers, Kubernetes)
  kubedojo doctor    Check that this machine can run sandboxes
  kubedojo down      Remove every sandbox kubedojo created
  kubedojo content lint          Validate the content
  kubedojo content test          Prove every question is solvable (needs Docker)
  kubedojo version

Run "kubedojo <command> -h" for flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	switch os.Args[1] {
	case "up":
		err = cmdServe(ctx, os.Args[2:], true)
	case "serve":
		err = cmdServe(ctx, os.Args[2:], false)
	case "doctor":
		err = cmdDoctor(ctx, os.Args[2:])
	case "down":
		err = cmdDown(ctx, os.Args[2:])
	case "content":
		if len(os.Args) < 3 {
			fmt.Print(usage)
			os.Exit(2)
		}
		switch os.Args[2] {
		case "lint":
			err = cmdLint(os.Args[3:])
		case "test":
			err = cmdTest(ctx, os.Args[3:])
		default:
			fmt.Print(usage)
			os.Exit(2)
		}
	case "version", "--version", "-v":
		fmt.Println("kubedojo", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// common holds flags shared by commands that touch sandboxes or content.
type common struct {
	contentDir string
	dataDir    string
	k8sVersion string
	nodeImage  string
	termImage  string
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil {
		return d
	}
	return def
}

func (c *common) register(fs *flag.FlagSet) {
	home, _ := os.UserHomeDir()
	fs.StringVar(&c.contentDir, "content", env("KUBEDOJO_CONTENT", ""), "load content from this directory instead of the built-in content")
	fs.StringVar(&c.dataDir, "data", env("KUBEDOJO_DATA", filepath.Join(home, ".kubedojo")), "directory for the database")
	fs.StringVar(&c.k8sVersion, "kubernetes-version", env("KUBEDOJO_K8S_VERSION", "v1.36.1"), "Kubernetes version of the sandbox clusters")
	fs.StringVar(&c.nodeImage, "node-image", env("KUBEDOJO_NODE_IMAGE", ""), "node image (default kubedojo/node:<kubernetes-version>)")
	fs.StringVar(&c.termImage, "terminal-image", env("KUBEDOJO_TERMINAL_IMAGE", ""), "terminal image (default kubedojo/terminal:<kubernetes-version>)")
}

func (c *common) driver() *sandbox.Kind {
	if c.nodeImage == "" {
		c.nodeImage = "kubedojo/node:" + c.k8sVersion
	}
	if c.termImage == "" {
		c.termImage = "kubedojo/terminal:" + c.k8sVersion
	}
	return &sandbox.Kind{NodeImage: c.nodeImage, TerminalImage: c.termImage}
}

func (c *common) contentFS() fs.FS {
	if c.contentDir != "" {
		return os.DirFS(c.contentDir)
	}
	return content.FS
}

func (c *common) openStore() (*store.Store, error) {
	if err := os.MkdirAll(c.dataDir, 0o755); err != nil {
		return nil, err
	}
	return store.Open(filepath.Join(c.dataDir, "kubedojo.db"))
}

func cmdServe(ctx context.Context, args []string, local bool) error {
	fl := flag.NewFlagSet("serve", flag.ContinueOnError)
	var c common
	c.register(fl)
	addr := fl.String("addr", env("KUBEDOJO_ADDR", "127.0.0.1:4400"), "listen address")
	noOpen := fl.Bool("no-open", false, "do not open the browser")
	maxSessions := fl.Int("max-sessions", 3, "maximum number of live sessions")
	userHeader := fl.String("user-header", env("KUBEDOJO_USER_HEADER", ""), "trusted header carrying the user name (behind an OIDC proxy)")
	hosts := fl.String("sandbox-host", env("KUBEDOJO_SANDBOX_HOST", "local"), "where sandboxes run: local | kubernetes")
	idleTTL := fl.Duration("idle-ttl", envDuration("KUBEDOJO_IDLE_TTL", 0), "remove sandboxes that were not used for this long (0 = never)")
	observability := fl.Bool("observability", false, "start a local Grafana LGTM stack and send traces, metrics and logs to it")
	devOrigin := fl.String("dev-origin", env("KUBEDOJO_DEV_ORIGIN", ""), "extra allowed WebSocket origin (UI dev server)")
	if err := fl.Parse(args); err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if !local {
		log = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}

	if *observability {
		if err := startObservability(ctx, c.dataDir); err != nil {
			return fmt.Errorf("observability: %w", err)
		}
	}
	tel, err := telemetry.Setup(ctx, version, log)
	if err != nil {
		return fmt.Errorf("telemetry: %w", err)
	}
	defer tel.Shutdown()
	log = tel.Logger
	if tel.Enabled {
		log.Info("telemetry enabled", "endpoint", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	}

	cat, err := catalog.Load(c.contentFS())
	if err != nil {
		if len(cat.Tracks) == 0 {
			return fmt.Errorf("content: %w", err)
		}
		log.Warn("content has problems; run `kubedojo content lint`", "err", err)
	}
	drv := c.driver()
	if local {
		if err := preflight(ctx, drv, true); err != nil {
			return err
		}
	}
	st, err := c.openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	hp, err := hostProvider(ctx, *hosts, log)
	if err != nil {
		return err
	}
	mgr, err := session.NewManager(session.Config{Catalog: cat, Store: st, Driver: drv, Hosts: hp, Logger: log, MaxSessions: *maxSessions, IdleTTL: *idleTTL, Hooks: telemetryHooks(tel)})
	if err != nil {
		return err
	}
	defer mgr.Close()
	tel.ObserveSessions(mgr.ActiveCount)

	srv := &api.Server{Catalog: cat, Sessions: mgr, Store: st, UI: webui.FS(), Log: log, Version: version,
		UserHeader: *userHeader, OnTerminal: tel.Terminal, Tutor: newTutor(log)}
	if *devOrigin != "" {
		srv.AllowedOrigins = []string{*devOrigin}
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	hs := &http.Server{Handler: tel.HTTP(srv.Handler()), ReadHeaderTimeout: 10 * time.Second}
	url := "http://" + ln.Addr().String()
	log.Info("kubedojo is running", "url", url, "version", version)
	if local {
		fmt.Printf("\n  kubedojo is running at %s\n  Press Ctrl+C to stop. Sandboxes stay until you end the session or run `kubedojo down`.\n\n", url)
		if !*noOpen {
			openBrowser(url)
		}
	}
	errc := make(chan error, 1)
	go func() { errc <- hs.Serve(ln) }()
	select {
	case <-ctx.Done():
	case err := <-errc:
		return err
	}
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return hs.Shutdown(sctx)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func cmdLint(args []string) error {
	fl := flag.NewFlagSet("content lint", flag.ContinueOnError)
	var c common
	c.register(fl)
	if err := fl.Parse(args); err != nil {
		return err
	}
	cat, err := catalog.Load(c.contentFS())
	for _, t := range cat.List() {
		perDomain := map[string]int{}
		for _, q := range t.Questions {
			perDomain[q.Domain]++
		}
		fmt.Printf("%s — %s: %d questions\n", t.ID, t.Title, len(t.Questions))
		for _, d := range t.Domains {
			fmt.Printf("  %-28s %3d%%  %2d questions\n", d.ID, d.Weight, perDomain[d.ID])
		}
	}
	if err != nil {
		return fmt.Errorf("content is invalid:\n%w", err)
	}
	fmt.Println("content is valid")
	return nil
}
