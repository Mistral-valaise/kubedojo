package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kubedojo/kubedojo/images"
	"github.com/kubedojo/kubedojo/internal/dockerhost"
	"github.com/kubedojo/kubedojo/internal/sandbox"
)

func cmdDoctor(ctx context.Context, args []string) error {
	fl := flag.NewFlagSet("doctor", flag.ContinueOnError)
	var c common
	c.register(fl)
	build := fl.Bool("build", false, "build missing sandbox images")
	if err := fl.Parse(args); err != nil {
		return err
	}
	return preflight(ctx, c.driver(), *build)
}

func out(ctx context.Context, name string, args ...string) (string, error) {
	b, err := dockerhost.Local().Command(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(b)), err
}

// preflight verifies the machine can run sandboxes and (optionally) builds the
// sandbox images when they are missing.
func preflight(ctx context.Context, drv *sandbox.Kind, build bool) error {
	ok := func(msg string) { fmt.Println("  ✓", msg) }
	warn := func(msg string) { fmt.Println("  !", msg) }

	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not found. Install Docker Desktop, OrbStack or Colima")
	}
	info, err := out(ctx, "docker", "info", "--format", "{{.ServerVersion}} {{.NCPU}} {{.MemTotal}}")
	if err != nil {
		return fmt.Errorf("cannot reach the Docker daemon. Is Docker running?\n%s", info)
	}
	f := strings.Fields(info)
	ok("Docker " + f[0] + " is running")
	if len(f) == 3 {
		cpus, _ := strconv.Atoi(f[1])
		mem, _ := strconv.ParseFloat(f[2], 64)
		gb := mem / (1 << 30)
		switch {
		case gb < 3.5:
			warn(fmt.Sprintf("Docker has only %.1f GB memory. Sandboxes need at least 4 GB; 8 GB is recommended for exams", gb))
		case gb < 7:
			warn(fmt.Sprintf("Docker has %.1f GB memory. Practice works; a full exam (2 clusters) is more comfortable with 8 GB", gb))
		default:
			ok(fmt.Sprintf("Docker has %.1f GB memory and %d CPUs", gb, cpus))
		}
	}
	if _, err := exec.LookPath("kind"); err != nil {
		return fmt.Errorf("kind not found. Install it: `brew install kind` (macOS) or see https://kind.sigs.k8s.io/docs/user/quick-start/#installation")
	}
	v, _ := out(ctx, "kind", "version")
	ok(v)

	for _, img := range []struct{ tag, dir string }{{drv.NodeImage, "node"}, {drv.TerminalImage, "terminal"}} {
		if _, err := out(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", img.tag); err == nil {
			ok("image " + img.tag)
			continue
		}
		if _, err := out(ctx, "docker", "pull", "-q", img.tag); err == nil {
			ok("image " + img.tag + " (pulled)")
			continue
		}
		if !build {
			warn("image " + img.tag + " is missing. Run `kubedojo doctor --build` (takes a few minutes, once)")
			continue
		}
		fmt.Printf("  … building %s (first run only, takes a few minutes)\n", img.tag)
		if err := buildImage(ctx, img.tag, img.dir); err != nil {
			return err
		}
		ok("image " + img.tag + " (built)")
	}
	return nil
}

func buildImage(ctx context.Context, tag, dir string) error {
	tmp, err := os.MkdirTemp("", "kubedojo-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	sub, _ := fs.Sub(images.FS, dir)
	if err := os.CopyFS(tmp, sub); err != nil {
		return err
	}
	_ = os.Chmod(filepath.Join(tmp, "kubedojo-node-init"), 0o755)
	k8s := tag[strings.LastIndex(tag, ":")+1:]
	cmd := dockerhost.Local().Command(ctx, "docker", "build", "-t", tag, "--build-arg", "K8S_VERSION="+k8s, tmp)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

// cmdDown removes every sandbox: kind clusters whose nodes carry kubedojo's
// image, and terminal containers.
func cmdDown(ctx context.Context, args []string) error {
	fl := flag.NewFlagSet("down", flag.ContinueOnError)
	var c common
	c.register(fl)
	if err := fl.Parse(args); err != nil {
		return err
	}
	drv := c.driver()
	ids, _ := out(ctx, "docker", "ps", "-aq", "--filter", "label=kubedojo.session")
	for _, id := range strings.Fields(ids) {
		_, _ = out(ctx, "docker", "rm", "-f", id)
	}
	fmt.Printf("removed %d terminal container(s)\n", len(strings.Fields(ids)))
	names, _ := out(ctx, "docker", "ps", "-a", "--filter", "ancestor="+drv.NodeImage, "--format", `{{.Label "io.x-k8s.kind.cluster"}}`)
	seen := map[string]bool{}
	for _, n := range strings.Fields(names) {
		if seen[n] {
			continue
		}
		seen[n] = true
		if _, err := out(ctx, "kind", "delete", "cluster", "--name", n); err != nil {
			fmt.Println("could not delete cluster", n)
		} else {
			fmt.Println("deleted cluster", n)
		}
	}
	if st, err := c.openStore(); err == nil {
		defer st.Close()
		_ = st.MarkAllDestroyed()
	}
	return nil
}
