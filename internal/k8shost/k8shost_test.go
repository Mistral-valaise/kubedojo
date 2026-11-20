package k8shost

import (
	"encoding/json"
	"strings"
	"testing"
)

func valid() Config {
	return Config{Namespace: "kubedojo", Image: "sandbox-host:v1", CPURequest: "1", CPULimit: "4",
		MemRequest: "3Gi", MemLimit: "8Gi", EphemeralRequest: "10Gi", EphemeralLimit: "30Gi"}
}

func TestQuantity(t *testing.T) {
	for in, want := range map[string]float64{
		"1": 1, "500m": 0.5, "2.5": 2.5, "3Gi": 3 * gi, "512Mi": 512 << 20, "10G": 10e9, "1Ti": 1 << 40, "64Ki": 64 << 10, "2k": 2000,
	} {
		got, err := quantity(in)
		if err != nil || got != want {
			t.Errorf("quantity(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "3GB", "lots", "-1", "1e3", "3 Gi", "Gi", "1.2.3"} {
		if v, err := quantity(in); err == nil {
			t.Errorf("quantity(%q) = %v, want an error", in, v)
		}
	}
}

func TestValidate(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatalf("the defaults are rejected: %v", err)
	}
	// Limits are optional; a sandbox without them may use what the node has.
	open := valid()
	open.CPULimit, open.MemLimit, open.EphemeralLimit = "", "", ""
	if err := open.Validate(); err != nil {
		t.Fatalf("a sandbox without limits is rejected: %v", err)
	}
	for name, c := range map[string]struct {
		change func(*Config)
		want   string
	}{
		"request above limit":      {func(c *Config) { c.MemRequest = "9Gi" }, "above its limit"},
		"cpu request above limit":  {func(c *Config) { c.CPURequest = "8" }, "above its limit"},
		"not a quantity":           {func(c *Config) { c.MemLimit = "8GB" }, "not a quantity"},
		"no request":               {func(c *Config) { c.EphemeralRequest = "" }, "request is not set"},
		"memory too small":         {func(c *Config) { c.MemRequest, c.MemLimit = "256Mi", "512Mi" }, "too small"},
		"disk too small":           {func(c *Config) { c.EphemeralRequest, c.EphemeralLimit = "1Gi", "2Gi" }, "too small"},
		"too small without limit":  {func(c *Config) { c.MemRequest, c.MemLimit = "512Mi", "" }, "too small"},
		"a typo that takes a node": {func(c *Config) { c.MemLimit = "800Gi" }, "above the maximum"},
		"cpu above the maximum":    {func(c *Config) { c.CPULimit = "400" }, "above the maximum"},
	} {
		cfg := valid()
		c.change(&cfg)
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Validate() = %v, want an error with %q", name, err, c.want)
		}
	}
}

// The Pod is what the cluster's scheduler and kubelet act on. Read it back as
// they would.
func TestManifestResources(t *testing.T) {
	type pod struct {
		Spec struct {
			Containers []struct {
				Resources struct {
					Requests map[string]string `json:"requests"`
					Limits   map[string]string `json:"limits"`
				} `json:"resources"`
				SecurityContext struct {
					Privileged bool `json:"privileged"`
				} `json:"securityContext"`
			} `json:"containers"`
			Volumes []struct {
				Name     string `json:"name"`
				EmptyDir *struct {
					SizeLimit string `json:"sizeLimit"`
				} `json:"emptyDir"`
			} `json:"volumes"`
			AutomountServiceAccountToken *bool `json:"automountServiceAccountToken"`
		} `json:"spec"`
	}
	render := func(c Config) pod {
		raw, err := json.Marshal((&Provider{cfg: c}).manifest("kd-sandbox-abc", "abc"))
		if err != nil {
			t.Fatal(err)
		}
		var p pod
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	sizeLimit := func(p pod) string {
		for _, v := range p.Spec.Volumes {
			if v.Name == "docker" && v.EmptyDir != nil {
				return v.EmptyDir.SizeLimit
			}
		}
		t.Fatal("the Pod has no emptyDir for Docker's data")
		return ""
	}

	p := render(valid())
	res := p.Spec.Containers[0].Resources
	for k, want := range map[string]string{"cpu": "1", "memory": "3Gi", "ephemeral-storage": "10Gi"} {
		if res.Requests[k] != want {
			t.Errorf("request %s = %q, want %q", k, res.Requests[k], want)
		}
	}
	for k, want := range map[string]string{"cpu": "4", "memory": "8Gi", "ephemeral-storage": "30Gi"} {
		if res.Limits[k] != want {
			t.Errorf("limit %s = %q, want %q", k, res.Limits[k], want)
		}
	}
	if got := sizeLimit(p); got != "30Gi" {
		t.Errorf("the Docker volume is limited to %q, want the ephemeral storage limit 30Gi", got)
	}
	if p.Spec.AutomountServiceAccountToken == nil || *p.Spec.AutomountServiceAccountToken {
		t.Error("a sandbox Pod must not get a ServiceAccount token")
	}

	// No limits: none in the Pod, and no size limit on the volume. An empty
	// string as a limit would be rejected by the API server.
	open := valid()
	open.CPULimit, open.MemLimit, open.EphemeralLimit = "", "", ""
	p = render(open)
	if l := p.Spec.Containers[0].Resources.Limits; len(l) != 0 {
		t.Errorf("limits without any configured: %v", l)
	}
	if got := sizeLimit(p); got != "" {
		t.Errorf("size limit without an ephemeral storage limit: %q", got)
	}
}
