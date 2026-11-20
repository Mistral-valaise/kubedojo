package k8shost

import (
	"fmt"
	"strconv"
	"strings"
)

// quantity parses a Kubernetes resource quantity ("500m", "2", "3Gi", "10G")
// into its value: cores for CPU, bytes for memory and storage. It knows the
// suffixes sandbox resources are written with, not the whole grammar: no
// exponents.
func quantity(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty quantity")
	}
	suffixes := []struct {
		s string
		f float64
	}{
		{"Ki", 1 << 10}, {"Mi", 1 << 20}, {"Gi", 1 << 30}, {"Ti", 1 << 40},
		{"m", 1e-3}, {"k", 1e3}, {"M", 1e6}, {"G", 1e9}, {"T", 1e12},
	}
	num, factor := s, 1.0
	for _, x := range suffixes {
		if strings.HasSuffix(s, x.s) {
			num, factor = strings.TrimSuffix(s, x.s), x.f
			break
		}
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil || v < 0 || strings.ContainsAny(num, "eE+- ") {
		return 0, fmt.Errorf("%q is not a quantity like 500m, 2, 3Gi or 10G", s)
	}
	return v * factor, nil
}

// Quantity parses a Kubernetes resource quantity: cores for CPU, bytes for
// memory and storage.
func Quantity(s string) (float64, error) { return quantity(s) }

const gi = float64(1 << 30)

// Bounds of what one sandbox may ask for. The lower ones are what a sandbox
// needs to work at all, the upper ones catch a typo before it reserves a node.
const (
	minMemory    = 1 * gi   // a single-node cluster does not start with less
	minEphemeral = 8 * gi   // the preloaded images alone take about 5 GB of it
	maxCPU       = 64.0     // cores
	maxMemory    = 256 * gi // per sandbox
	maxEphemeral = 1024 * gi
)

// Validate checks the resources of a sandbox Pod: every quantity parses, a
// request is not above its limit, and the values are within the bounds above.
// An empty limit means "no limit" and is allowed; requests are required,
// because without them the scheduler packs sandboxes onto a node that cannot
// hold them.
func (c Config) Validate() error {
	type pair struct {
		what, request, limit string
		min, max             float64
		unit                 string
	}
	for _, p := range []pair{
		{"CPU", c.CPURequest, c.CPULimit, 0.1, maxCPU, "cores"},
		{"memory", c.MemRequest, c.MemLimit, minMemory, maxMemory, "bytes"},
		{"ephemeral storage", c.EphemeralRequest, c.EphemeralLimit, minEphemeral, maxEphemeral, "bytes"},
	} {
		if p.request == "" {
			return fmt.Errorf("sandbox %s request is not set", p.what)
		}
		req, err := quantity(p.request)
		if err != nil {
			return fmt.Errorf("sandbox %s request: %w", p.what, err)
		}
		// The most a sandbox may use: its limit, or its request without one.
		top, name := req, "request"
		if p.limit != "" {
			lim, err := quantity(p.limit)
			if err != nil {
				return fmt.Errorf("sandbox %s limit: %w", p.what, err)
			}
			if req > lim {
				return fmt.Errorf("sandbox %s request %s is above its limit %s", p.what, p.request, p.limit)
			}
			top, name = lim, "limit"
		}
		if top < p.min {
			return fmt.Errorf("sandbox %s %s is too small for a sandbox to work: at least %s", p.what, name, human(p.min, p.unit))
		}
		if top > p.max {
			return fmt.Errorf("sandbox %s %s is above the maximum of %s", p.what, name, human(p.max, p.unit))
		}
	}
	return nil
}

func human(v float64, unit string) string {
	if unit == "bytes" {
		return strconv.FormatFloat(v/gi, 'f', -1, 64) + "Gi"
	}
	return strconv.FormatFloat(v, 'f', -1, 64) + " " + unit
}
