package sandbox

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
)

// Usage is what the sandbox containers of one role use together.
type Usage struct {
	Containers  int
	MemoryBytes int64
	// CPU is in cores: 1.5 is one and a half cores busy.
	CPU float64
}

// Roles of sandbox containers in Usage.
const (
	UsageNode     = "node"     // a node of a kind cluster
	UsageTerminal = "terminal" // the terminal of a session with clusters
	UsageLinux    = "linux"    // the host of a track without a cluster
)

// Usage samples the memory and CPU of the sandbox containers on a Docker
// endpoint, by role, and returns the memory Docker has in total. It asks
// `docker stats` once, which takes about two seconds: call it from a ticker,
// not from a request.
func (k *Kind) Usage(ctx context.Context, h dockerhost.Host) (map[string]Usage, int64, error) {
	out, err := run(h.Command(ctx, "docker", "ps", "--no-trunc", "--format", `{{.ID}}\t{{.Image}}`), "")
	if err != nil {
		return nil, 0, err
	}
	roles := map[string]string{}
	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		id, image, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		role := k.usageRole(image)
		if role == "" {
			continue
		}
		roles[id] = role
		ids = append(ids, id)
	}
	usage := map[string]Usage{}
	if len(ids) > 0 {
		args := append([]string{"stats", "--no-stream", "--no-trunc", "--format", `{{.ID}}\t{{.CPUPerc}}\t{{.MemUsage}}`}, ids...)
		out, err := run(h.Command(ctx, "docker", args...), "")
		if err != nil {
			return nil, 0, err
		}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			f := strings.Split(line, "\t")
			if len(f) != 3 {
				continue
			}
			role, ok := roles[f[0]]
			if !ok {
				continue
			}
			u := usage[role]
			u.Containers++
			u.CPU += parsePercent(f[1]) / 100
			used, _, _ := strings.Cut(f[2], "/")
			u.MemoryBytes += parseBytes(used)
			usage[role] = u
		}
	}
	total := int64(0)
	if out, err := run(h.Command(ctx, "docker", "info", "--format", "{{.MemTotal}}"), ""); err == nil {
		total, _ = strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	}
	return usage, total, nil
}

// usageRole tells a sandbox container from everything else on the machine by
// its image: only kubedojo runs these.
func (k *Kind) usageRole(image string) string {
	switch image {
	case "":
		return ""
	case k.NodeImage:
		return UsageNode
	case k.TerminalImage:
		return UsageTerminal
	case k.LinuxImage:
		return UsageLinux
	}
	return ""
}

// parsePercent reads the CPU column of docker stats: "12.34%".
func parsePercent(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "%"), 64)
	return f
}

// parseBytes reads a size as docker stats prints it: "512B", "12.5MiB",
// "1.2GiB", and the decimal units "kB", "MB", "GB". Unknown input is 0.
func parseBytes(s string) int64 {
	s = strings.TrimSpace(s)
	i := strings.IndexFunc(s, func(r rune) bool { return (r < '0' || r > '9') && r != '.' })
	if i <= 0 {
		return 0
	}
	n, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0
	}
	mult, ok := map[string]float64{
		"B": 1, "kB": 1e3, "KB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12,
		"KiB": 1 << 10, "MiB": 1 << 20, "GiB": 1 << 30, "TiB": 1 << 40,
	}[strings.TrimSpace(s[i:])]
	if !ok {
		return 0
	}
	return int64(n * mult)
}

func (u Usage) String() string {
	return fmt.Sprintf("%d containers, %d bytes, %.2f cores", u.Containers, u.MemoryBytes, u.CPU)
}
