package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Mistral-valaise/kubedojo/images"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
)

// publishedImages is where the release workflow pushes the sandbox images.
const publishedImages = "ghcr.io/mistral-valaise"

// defaultLinuxImage is the lab host of container environments. Its tag names
// the Debian release: it has nothing to do with the Kubernetes version.
const defaultLinuxImage = "kubedojo/linux:trixie"

// needs says which kinds of sandbox a command is going to create, and with
// that what the machine must provide. A track without clusters runs on a
// machine that has Docker and nothing else.
type needs struct {
	cluster   bool // kind, the node image
	container bool // the linux image
}

// everything is what a server needs: it may be asked for any track.
var everything = needs{cluster: true, container: true}

// needsOf reports what the sandboxes of the given tracks need.
func needsOf(tracks []*catalog.Track) needs {
	var n needs
	for _, t := range tracks {
		n.cluster = n.cluster || t.NeedsCluster()
		n.container = n.container || t.NeedsContainer()
	}
	return n
}

// imagesFor lists the images to check as {tag, build context} pairs. The
// terminal is part of every sandbox.
func imagesFor(drv *sandbox.Kind, n needs) [][2]string {
	var out [][2]string
	if n.cluster {
		out = append(out, [2]string{drv.NodeImage, "node"})
	}
	if n.cluster || n.container {
		out = append(out, [2]string{drv.TerminalImage, "terminal"})
	}
	if n.container {
		out = append(out, [2]string{drv.LinuxImage, "linux"})
	}
	return out
}

func cmdDoctor(ctx context.Context, args []string) error {
	fl := flag.NewFlagSet("doctor", flag.ContinueOnError)
	var c common
	c.register(fl)
	build := fl.Bool("build", false, "build missing sandbox images")
	track := fl.String("track", "", "check only what this track needs")
	if err := fl.Parse(args); err != nil {
		return err
	}
	n := everything
	if *track != "" {
		cat, _ := catalog.Load(c.contentFS())
		t := cat.Tracks[*track]
		if t == nil {
			return fmt.Errorf("unknown track %q", *track)
		}
		n = needsOf([]*catalog.Track{t})
	}
	return preflight(ctx, c.driver(), *build, n)
}

func out(ctx context.Context, name string, args ...string) (string, error) {
	b, err := dockerhost.Local().Command(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(b)), err
}

// preflight verifies the machine can run the sandboxes n asks for and
// (optionally) builds their images when they are missing.
func preflight(ctx context.Context, drv *sandbox.Kind, build bool, n needs) error {
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
		case !n.cluster:
			// The warnings below are about clusters; a Linux host is capped at 1 GB.
			ok(fmt.Sprintf("Docker has %.1f GB memory and %d CPUs", gb, cpus))
		case gb < 3.5:
			warn(fmt.Sprintf("Docker has only %.1f GB memory. Sandboxes need at least 4 GB; 8 GB is recommended for exams", gb))
		case gb < 7:
			warn(fmt.Sprintf("Docker has %.1f GB memory. Practice works; a full exam (2 clusters) is more comfortable with 8 GB", gb))
		default:
			ok(fmt.Sprintf("Docker has %.1f GB memory and %d CPUs", gb, cpus))
		}
	}
	if n.cluster {
		if _, err := exec.LookPath("kind"); err != nil {
			return fmt.Errorf("kind not found. Install it: `brew install kind` (macOS) or see https://kind.sigs.k8s.io/docs/user/quick-start/#installation")
		}
		v, _ := out(ctx, "kind", "version")
		ok(v)

		// Several kind clusters need more inotify instances than most Linux
		// distributions allow by default; the symptom is a second cluster that
		// fails to start.
		if b, err := os.ReadFile("/proc/sys/fs/inotify/max_user_instances"); err == nil {
			if n, _ := strconv.Atoi(strings.TrimSpace(string(b))); n > 0 && n < 512 {
				warn(fmt.Sprintf("fs.inotify.max_user_instances is %d. Exams create several clusters; run: sudo sysctl -w fs.inotify.max_user_instances=512 fs.inotify.max_user_watches=524288", n))
			} else {
				ok("inotify limits are sufficient")
			}
		}
	}

	for _, pair := range imagesFor(drv, n) {
		img := struct{ tag, dir string }{pair[0], pair[1]}
		// Only kubedojo's own tags have a revision and a published copy: an
		// image passed with --node-image is the user's business.
		own := strings.HasPrefix(img.tag, "kubedojo/"+img.dir+":")
		current := func(ref string) (present, fresh bool) {
			have, err := out(ctx, "docker", "image", "inspect", "--format", `{{index .Config.Labels "`+images.RevLabel+`"}}`, ref)
			return err == nil, err == nil && (!own || have == images.Rev(img.dir))
		}
		present, fresh := current(img.tag)
		if fresh {
			ok("image " + img.tag)
			continue
		}
		if own {
			// Releases publish the images; a copy of the right revision saves the build.
			published := publishedImages + "/kubedojo-" + img.dir + img.tag[strings.LastIndex(img.tag, ":"):]
			if _, err := out(ctx, "docker", "pull", "-q", published); err == nil {
				if _, fresh := current(published); fresh {
					if _, err := out(ctx, "docker", "tag", published, img.tag); err == nil {
						ok("image " + img.tag + " (pulled)")
						continue
					}
				}
			}
		} else if _, err := out(ctx, "docker", "pull", "-q", img.tag); err == nil {
			ok("image " + img.tag + " (pulled)")
			continue
		}
		if !build {
			if present {
				warn("image " + img.tag + " is from an older kubedojo and lacks tools newer questions need. Run `kubedojo doctor --build`")
			} else {
				warn("image " + img.tag + " is missing. Run `kubedojo doctor --build` (takes a few minutes, once)")
			}
			continue
		}
		if present {
			fmt.Printf("  … rebuilding %s: this kubedojo ships a newer version of it (takes a few minutes)\n", img.tag)
		} else {
			fmt.Printf("  … building %s (first run only, takes a few minutes)\n", img.tag)
		}
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
	args := []string{"build", "-t", tag}
	// The tag of the node and terminal images is the Kubernetes version they
	// are built for; the linux image has no such argument.
	if df, _ := fs.ReadFile(sub, "Dockerfile"); bytes.Contains(df, []byte("ARG K8S_VERSION")) {
		args = append(args, "--build-arg", "K8S_VERSION="+tag[strings.LastIndex(tag, ":")+1:])
	}
	cmd := dockerhost.Local().Command(ctx, "docker", append(args, tmp)...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

// cmdDown removes every sandbox: kind clusters whose nodes carry kubedojo's
// image, and the containers and networks that carry a session label
// (terminals, Linux hosts and the bridges between them).
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
	fmt.Printf("removed %d sandbox container(s)\n", len(strings.Fields(ids)))
	// After the containers: Docker keeps a network that still has endpoints.
	nets, _ := out(ctx, "docker", "network", "ls", "-q", "--filter", "label=kubedojo.session")
	for _, id := range strings.Fields(nets) {
		_, _ = out(ctx, "docker", "network", "rm", id)
	}
	// The cache container goes too; its volume (the downloaded images) stays.
	_, _ = out(ctx, "docker", "rm", "-f", "kubedojo-registry-cache")
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
