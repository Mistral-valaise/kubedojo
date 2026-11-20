package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/observability"
)

// obsFlags are the flags of the local observability stack. `up` and `serve`
// have them next to --observability; `kubedojo observability` has them alone.
type obsFlags struct {
	enabled     *bool
	grafanaPort *int
	otlpPort    *int
	ephemeral   *bool
	retention   *time.Duration
	image       *string
}

func envBool(key string, def bool) bool {
	if b, err := strconv.ParseBool(os.Getenv(key)); err == nil {
		return b
	}
	return def
}

func (f *obsFlags) register(fl *flag.FlagSet, withSwitch bool) {
	on := true
	f.enabled = &on
	if withSwitch {
		f.enabled = fl.Bool("observability", envBool("KUBEDOJO_OBSERVABILITY", false), "start a local Grafana stack (Grafana, Prometheus, Loki, Tempo, OpenTelemetry Collector) and send traces, metrics and logs to it")
	}
	f.grafanaPort = fl.Int("grafana-port", envInt("KUBEDOJO_GRAFANA_PORT", observability.DefaultGrafanaPort), "observability: local port of Grafana")
	f.otlpPort = fl.Int("otlp-port", envInt("KUBEDOJO_OTLP_PORT", observability.DefaultOTLPPort), "observability: local port of the OTLP/HTTP receiver")
	f.ephemeral = fl.Bool("observability-ephemeral", envBool("KUBEDOJO_OBSERVABILITY_EPHEMERAL", false), "observability: keep no data volume; metrics, logs and traces are gone when the stack restarts")
	f.retention = fl.Duration("observability-retention", envDuration("KUBEDOJO_OBSERVABILITY_RETENTION", observability.DefaultRetention), "observability: how long metrics, logs and traces are kept (at least 24h)")
	f.image = fl.String("observability-image", env("KUBEDOJO_OBSERVABILITY_IMAGE", observability.Image), "observability: the grafana/otel-lgtm image to run")
}

func (f *obsFlags) stack(dataDir string) (*observability.Stack, error) {
	s, err := observability.New(observability.Options{DataDir: dataDir, Image: *f.image, GrafanaPort: *f.grafanaPort,
		OTLPPort: *f.otlpPort, Ephemeral: *f.ephemeral, Retention: *f.retention, Version: version}, out)
	if err != nil {
		return nil, err
	}
	s.Out = os.Stdout
	return s, nil
}

// verifyStack prints what Verify found and fails if a check did.
func verifyStack(ctx context.Context, stack *observability.Stack) error {
	fmt.Println("\nVerifying: a trace, a log record and a metric go in over OTLP and are read back through Grafana.")
	checks := stack.Verify(ctx, 45*time.Second)
	for _, c := range checks {
		if c.Err != nil {
			fmt.Printf("  ✗ %s\n      %v\n", c.Name, c.Err)
		} else {
			fmt.Printf("  ✓ %s\n", c.Name)
		}
	}
	if n := len(observability.Failed(checks)); n > 0 {
		return fmt.Errorf("%d of %d checks failed", n, len(checks))
	}
	return nil
}

const observabilityUsage = `The local observability stack: Grafana, Prometheus, Loki, Tempo and an OpenTelemetry Collector
in one container. "kubedojo up --observability" starts it for you.

Usage:
  kubedojo observability up       Start the stack, or keep the running one
  kubedojo observability status   Show whether it runs, and where. --verify proves that it works:
                                  a trace, a log record and a metric are written and read back.
  kubedojo observability stop     Stop it; metrics, logs and traces are kept
  kubedojo observability reset    Stop it and delete its data. Your progress is not touched.

Run "kubedojo observability <command> -h" for flags.
`

func cmdObservability(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(observabilityUsage)
		return nil
	}
	fl := flag.NewFlagSet("observability "+args[0], flag.ContinueOnError)
	var c common
	fl.StringVar(&c.dataDir, "data", env("KUBEDOJO_DATA", defaultDataDir()), "data directory")
	var f obsFlags
	f.register(fl, false)
	verify := fl.Bool("verify", false, "up, status: send a trace, a log record and a metric and read them back through Grafana")
	if err := fl.Parse(args[1:]); err != nil {
		return err
	}
	stack, err := f.stack(c.dataDir)
	if err != nil {
		return err
	}
	switch args[0] {
	case "up":
		if err := stack.Up(ctx); err != nil {
			return err
		}
		fmt.Printf("  ✓ Grafana: %s/dashboards?tag=kubedojo\n  ✓ OTLP/HTTP: %s\n", stack.GrafanaURL(), stack.Endpoint())
		fmt.Println("  Start kubedojo with --observability to send its telemetry here.")
		if *verify {
			return verifyStack(ctx, stack)
		}
	case "status":
		st, err := stack.Status(ctx)
		if err != nil {
			return err
		}
		switch {
		case !st.Exists:
			fmt.Println("The observability stack is not running.")
		case !st.Owned:
			fmt.Printf("A container named %s exists, but kubedojo did not create it.\n", observability.Container)
		case !st.Running:
			fmt.Println("The observability stack is stopped.")
		default:
			fmt.Printf("The observability stack is running.\n  image:    %s\n", strings.SplitN(st.Image, "@", 2)[0])
			if stack.Current(st) {
				fmt.Printf("  Grafana:  %s/dashboards?tag=kubedojo\n  OTLP:     %s\n", stack.GrafanaURL(), stack.Endpoint())
			} else {
				fmt.Println("  It was started with other settings than the ones given here; `up` restarts it with these.")
			}
		}
		if st.Volume {
			fmt.Printf("  data:     Docker volume %s (delete with: kubedojo observability reset)\n", observability.Volume)
		} else {
			fmt.Println("  data:     no volume")
		}
		if *verify {
			if !st.Running || !st.Owned {
				return fmt.Errorf("nothing to verify: the observability stack is not running")
			}
			return verifyStack(ctx, stack)
		}
	case "stop":
		removed, err := stack.Stop(ctx)
		if err != nil {
			return err
		}
		if !removed {
			fmt.Println("The observability stack was not running.")
			return nil
		}
		fmt.Printf("Stopped. Metrics, logs and traces are kept in the Docker volume %s.\n", observability.Volume)
	case "reset":
		if err := stack.Reset(ctx); err != nil {
			return err
		}
		fmt.Println("Removed the observability stack and its data. Your progress is untouched.")
	default:
		fmt.Print(observabilityUsage)
		os.Exit(2)
	}
	return nil
}
