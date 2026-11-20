package sandbox

import "testing"

func TestParseDockerStats(t *testing.T) {
	for in, want := range map[string]int64{
		"512B":      512,
		"12.5MiB":   13107200,
		"1.5GiB ":   1610612736,
		" 800KiB":   819200,
		"2kB":       2000,
		"3MB":       3000000,
		"0B":        0,
		"":          0,
		"--":        0,
		"12 parsec": 0,
	} {
		if got := parseBytes(in); got != want {
			t.Errorf("parseBytes(%q) = %d, want %d", in, got, want)
		}
	}
	for in, want := range map[string]float64{"12.34%": 12.34, "0.00%": 0, "150%": 150, "--": 0} {
		if got := parsePercent(in); got != want {
			t.Errorf("parsePercent(%q) = %v, want %v", in, got, want)
		}
	}
}

// Only kubedojo's own images count: the machine runs other containers too.
func TestUsageRole(t *testing.T) {
	k := &Kind{NodeImage: "kubedojo/node:v1", TerminalImage: "kubedojo/terminal:v1", LinuxImage: "kubedojo/linux:trixie"}
	for image, want := range map[string]string{
		"kubedojo/node:v1":         UsageNode,
		"kubedojo/terminal:v1":     UsageTerminal,
		"kubedojo/linux:trixie":    UsageLinux,
		"postgres:17":              "",
		"grafana/otel-lgtm:0.41.1": "",
		"":                         "",
	} {
		if got := k.usageRole(image); got != want {
			t.Errorf("usageRole(%q) = %q, want %q", image, got, want)
		}
	}
	// An image that is not configured must not match the empty string.
	if got := (&Kind{}).usageRole(""); got != "" {
		t.Errorf("an unconfigured driver matched an empty image as %q", got)
	}
}
