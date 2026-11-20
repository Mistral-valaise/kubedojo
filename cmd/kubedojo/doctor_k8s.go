package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/netip"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
	"github.com/Mistral-valaise/kubedojo/internal/k8shost"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// finding is one line of a preflight report.
type finding struct {
	level string // ok, warn or fail
	text  string
}

func report(findings []finding) error {
	failed := 0
	for _, f := range findings {
		switch f.level {
		case "ok":
			fmt.Println("  ✓", f.text)
		case "warn":
			fmt.Println("  !", f.text)
		default:
			fmt.Println("  ✗", f.text)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d check(s) failed", failed)
	}
	return nil
}

// kube runs kubectl against the chosen context.
type kube struct {
	run     func(ctx context.Context, name string, args ...string) (string, error)
	context string
}

func (k kube) kubectl(ctx context.Context, args ...string) (string, error) {
	if k.context != "" {
		args = append([]string{"--context", k.context}, args...)
	}
	return k.run(ctx, "kubectl", args...)
}

func (k kube) json(ctx context.Context, into any, args ...string) error {
	out, err := k.kubectl(ctx, append(args, "-o", "json")...)
	if err != nil {
		return fmt.Errorf("%s", lastLineOf(out, err))
	}
	return json.Unmarshal([]byte(out), into)
}

func lastLineOf(out string, err error) string {
	if out = strings.TrimSpace(out); out != "" {
		lines := strings.Split(out, "\n")
		return strings.TrimSpace(lines[len(lines)-1])
	}
	return err.Error()
}

// What kubectl returns, as far as the checks read it.
type (
	meta struct {
		Name        string            `json:"name"`
		Namespace   string            `json:"namespace"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	}
	nodeList struct {
		Items []struct {
			Metadata meta `json:"metadata"`
			Spec     struct {
				Unschedulable bool `json:"unschedulable"`
				Taints        []struct {
					Key    string `json:"key"`
					Effect string `json:"effect"`
				} `json:"taints"`
			} `json:"spec"`
			Status struct {
				Allocatable map[string]string `json:"allocatable"`
				NodeInfo    struct {
					Architecture            string `json:"architecture"`
					KernelVersion           string `json:"kernelVersion"`
					OSImage                 string `json:"osImage"`
					ContainerRuntimeVersion string `json:"containerRuntimeVersion"`
					KubeletVersion          string `json:"kubeletVersion"`
				} `json:"nodeInfo"`
				Addresses []struct {
					Type    string `json:"type"`
					Address string `json:"address"`
				} `json:"addresses"`
			} `json:"status"`
		} `json:"items"`
	}
	podList struct {
		Items []struct {
			Spec struct {
				NodeName   string `json:"nodeName"`
				Containers []struct {
					Resources struct {
						Requests map[string]string `json:"requests"`
					} `json:"resources"`
				} `json:"containers"`
			} `json:"spec"`
			Status struct {
				Phase string `json:"phase"`
			} `json:"status"`
		} `json:"items"`
	}
	classList struct {
		Items []struct {
			Metadata meta `json:"metadata"`
		} `json:"items"`
	}
)

// clusterNeeds is what the installation is going to ask of the cluster.
type clusterNeeds struct {
	sessions            int
	cpu, memory, disk   string // requests of one sandbox
	selector            map[string]string
	toleration          string
	sandboxArchitecture string // architecture of the sandbox host image; empty for any
}

// privateRanges are the destinations the chart's NetworkPolicy keeps sandboxes
// away from.
var privateRanges = func() []netip.Prefix {
	var out []netip.Prefix
	for _, c := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "100.64.0.0/10", "127.0.0.0/8"} {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}()

// checkNodes reads what the nodes are and whether the sandboxes fit on them.
func checkNodes(nodes nodeList, pods podList, need clusterNeeds) []finding {
	var out []finding
	add := func(level, format string, a ...any) { out = append(out, finding{level, fmt.Sprintf(format, a...)}) }
	q := func(s string) float64 { v, _ := k8shost.Quantity(s); return v }

	// What is already requested on each node.
	type used struct{ cpu, mem, disk float64 }
	requested := map[string]*used{}
	for _, p := range pods.Items {
		if p.Spec.NodeName == "" || p.Status.Phase == "Succeeded" || p.Status.Phase == "Failed" {
			continue
		}
		u := requested[p.Spec.NodeName]
		if u == nil {
			u = &used{}
			requested[p.Spec.NodeName] = u
		}
		for _, c := range p.Spec.Containers {
			u.cpu += q(c.Resources.Requests["cpu"])
			u.mem += q(c.Resources.Requests["memory"])
			u.disk += q(c.Resources.Requests["ephemeral-storage"])
		}
	}

	fits, eligible := 0, 0
	var public []string
	for _, n := range nodes.Items {
		info := n.Status.NodeInfo
		selected := true
		for k, v := range need.selector {
			selected = selected && n.Metadata.Labels[k] == v
		}
		tainted := ""
		for _, t := range n.Spec.Taints {
			if t.Effect == "NoSchedule" && t.Key != need.toleration {
				tainted = t.Key
			}
		}
		for _, a := range n.Status.Addresses {
			ip, err := netip.ParseAddr(a.Address)
			if err != nil || !ip.Is4() {
				continue
			}
			private := false
			for _, r := range privateRanges {
				private = private || r.Contains(ip)
			}
			if !private {
				public = append(public, a.Address+"/32")
			}
		}
		switch {
		case !selected:
			continue
		case n.Spec.Unschedulable:
			add("warn", "node %s is cordoned and takes no sandboxes", n.Metadata.Name)
			continue
		case tainted != "":
			add("warn", "node %s has the taint %s, which sandboxes do not tolerate (sandbox.tolerationKey)", n.Metadata.Name, tainted)
			continue
		}
		eligible++
		add("ok", "node %s: %s, kernel %s, %s, kubelet %s", n.Metadata.Name, info.Architecture, info.KernelVersion, info.ContainerRuntimeVersion, info.KubeletVersion)
		if need.sandboxArchitecture != "" && info.Architecture != need.sandboxArchitecture {
			add("fail", "node %s is %s, but the published sandbox host image is %s only. Build it for %s (make sandbox-host) and set sandbox.image, or use %s nodes",
				n.Metadata.Name, info.Architecture, need.sandboxArchitecture, info.Architecture, need.sandboxArchitecture)
		}
		u := requested[n.Metadata.Name]
		if u == nil {
			u = &used{}
		}
		alloc := n.Status.Allocatable
		free := [3]float64{q(alloc["cpu"]) - u.cpu, q(alloc["memory"]) - u.mem, q(alloc["ephemeral-storage"]) - u.disk}
		per := [3]float64{q(need.cpu), q(need.memory), q(need.disk)}
		names := [3]string{"CPU", "memory", "disk"}
		n1, short := math.MaxInt32, ""
		for i := range per {
			if per[i] <= 0 {
				continue
			}
			if c := int(math.Floor(free[i] / per[i])); c < n1 {
				n1, short = c, names[i]
			}
		}
		if n1 < 0 {
			n1 = 0
		}
		fits += n1
		add("ok", "node %s has room for %d sandbox(es): %.1f CPU, %.1f Gi memory, %.0f Gi disk free of requests; %s runs out first",
			n.Metadata.Name, n1, free[0], free[1]/(1<<30), free[2]/(1<<30), short)
	}
	switch {
	case eligible == 0:
		add("fail", "no node can take sandboxes: check sandbox.nodeSelector %v and the taints", need.selector)
	case fits < need.sessions:
		add("fail", "the cluster has room for %d sandbox(es), maxSessions is %d: each requests %s CPU, %s memory and %s disk. Lower maxSessions or the requests, or add capacity",
			fits, need.sessions, need.cpu, need.memory, need.disk)
	default:
		add("ok", "room for %d sandboxes, %d wanted (each requests %s CPU, %s memory, %s disk)", fits, need.sessions, need.cpu, need.memory, need.disk)
	}
	if len(public) > 0 {
		sort.Strings(public)
		add("warn", "nodes have public addresses, which the NetworkPolicy of the sandboxes does not block by itself. Set sandbox.networkPolicy.extraExceptCIDRs: [%s]", strings.Join(public, ", "))
	}
	return out
}

// checkCluster runs the checks that need the API server.
func checkCluster(ctx context.Context, k kube, namespace string, need clusterNeeds, deep bool) []finding {
	var out []finding
	add := func(level, format string, a ...any) { out = append(out, finding{level, fmt.Sprintf(format, a...)}) }

	v, err := k.kubectl(ctx, "get", "--raw", "/version")
	if err != nil {
		add("fail", "the cluster does not answer: %s", lastLineOf(v, err))
		return out
	}
	var ver struct {
		GitVersion string `json:"gitVersion"`
	}
	_ = json.Unmarshal([]byte(v), &ver)
	add("ok", "the cluster answers: Kubernetes %s", ver.GitVersion)

	var nodes nodeList
	var pods podList
	if err := k.json(ctx, &nodes, "get", "nodes"); err != nil {
		add("fail", "cannot list nodes: %v", err)
		return out
	}
	if err := k.json(ctx, &pods, "get", "pods", "--all-namespaces"); err != nil {
		add("warn", "cannot list Pods, so what is already reserved on the nodes is unknown: %v", err)
	}
	out = append(out, checkNodes(nodes, pods, need)...)

	// Storage for the history and the image cache.
	var classes classList
	if err := k.json(ctx, &classes, "get", "storageclass"); err != nil {
		add("warn", "cannot list StorageClasses: %v", err)
	} else {
		def := ""
		for _, c := range classes.Items {
			if c.Metadata.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
				def = c.Metadata.Name
			}
		}
		if def == "" {
			add("fail", "no default StorageClass: the history and the image cache need volumes. Set persistence.storageClass and registryCache.persistence.storageClass")
		} else {
			add("ok", "default StorageClass %s", def)
		}
	}

	// Ingress.
	if err := k.json(ctx, &classes, "get", "ingressclass"); err == nil {
		var names []string
		for _, c := range classes.Items {
			names = append(names, c.Metadata.Name)
		}
		if len(names) == 0 {
			add("warn", "no IngressClass: ingress.enabled will publish nothing. Without an ingress, reach kubedojo with kubectl port-forward")
		} else {
			add("ok", "ingress controller(s): %s", strings.Join(names, ", "))
		}
	}

	// The Namespace: may this account create privileged Pods in it?
	var ns struct {
		Metadata meta `json:"metadata"`
	}
	if err := k.json(ctx, &ns, "get", "namespace", namespace); err != nil {
		add("warn", "the Namespace %s does not exist yet. Create it and allow privileged Pods before the first session:\n      kubectl create namespace %s\n      kubectl label namespace %s pod-security.kubernetes.io/enforce=privileged", namespace, namespace, namespace)
	} else {
		level := ns.Metadata.Labels["pod-security.kubernetes.io/enforce"]
		switch level {
		case "privileged":
			add("ok", "the Namespace %s admits privileged Pods", namespace)
		case "":
			add("warn", "the Namespace %s has no Pod Security level; the cluster's default applies. If sandboxes are refused: kubectl label namespace %s pod-security.kubernetes.io/enforce=privileged", namespace, namespace)
		default:
			add("fail", "the Namespace %s enforces the Pod Security level %q, which refuses sandbox Pods: kubectl label namespace %s pod-security.kubernetes.io/enforce=privileged --overwrite", namespace, level, namespace)
		}
		// Ask the API server itself, admission included, without creating anything.
		manifest := fmt.Sprintf(`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"kubedojo-preflight","namespace":%q},"spec":{"containers":[{"name":"probe","image":"busybox","securityContext":{"privileged":true},"volumeMounts":[{"name":"m","mountPath":"/lib/modules"}]}],"volumes":[{"name":"m","hostPath":{"path":"/lib/modules"}}]}}`, namespace)
		if o, err := k.run(ctx, "sh", "-c", fmt.Sprintf("printf '%%s' '%s' | kubectl %s apply --dry-run=server -f -", manifest, contextFlag(k.context))); err != nil {
			add("fail", "the cluster refuses a privileged Pod with a hostPath in %s: %s", namespace, lastLineOf(o, err))
		} else {
			add("ok", "the API server admits a privileged Pod with /lib/modules in %s (dry run)", namespace)
		}
	}

	if deep {
		out = append(out, checkNetworkPolicy(ctx, k, namespace)...)
	} else {
		add("warn", "not checked: whether the CNI enforces NetworkPolicies, on which the sandboxes' Docker port depends. Run with --deep (it starts two small Pods)")
	}
	return out
}

func contextFlag(c string) string {
	if c == "" {
		return ""
	}
	return "--context " + c
}

// checkNetworkPolicy proves that the CNI enforces NetworkPolicies: a Pod that
// a policy isolates must not be reachable. It creates two Pods and a policy
// and removes them again.
func checkNetworkPolicy(ctx context.Context, k kube, namespace string) []finding {
	id := fmt.Sprintf("kubedojo-preflight-%04d", rand.IntN(10000))
	manifest := fmt.Sprintf(`
apiVersion: v1
kind: Pod
metadata: {name: %[1]s-target, namespace: %[2]s, labels: {kubedojo.dev/preflight: %[1]s}}
spec:
  terminationGracePeriodSeconds: 0
  containers: [{name: target, image: busybox:1.37, command: [sh, -c, "while true; do echo ok | nc -l -p 2375; done"]}]
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: %[1]s, namespace: %[2]s}
spec:
  podSelector: {matchLabels: {kubedojo.dev/preflight: %[1]s}}
  policyTypes: [Ingress]
`, id, namespace)
	apply := func(m string) (string, error) {
		f, err := os.CreateTemp("", "kubedojo-preflight-*.yaml")
		if err != nil {
			return "", err
		}
		defer os.Remove(f.Name())
		_, _ = f.WriteString(m)
		f.Close()
		return k.kubectl(ctx, "apply", "-f", f.Name())
	}
	cleanup := func() {
		c, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_, _ = kube{k.run, k.context}.kubectl(c, "-n", namespace, "delete", "pod", id+"-target", id+"-client", "--ignore-not-found", "--wait=false")
		_, _ = kube{k.run, k.context}.kubectl(c, "-n", namespace, "delete", "networkpolicy", id, "--ignore-not-found")
	}
	defer cleanup()
	if o, err := apply(manifest); err != nil {
		return []finding{{"warn", "could not test NetworkPolicy enforcement: " + lastLineOf(o, err)}}
	}
	if o, err := k.kubectl(ctx, "-n", namespace, "wait", "--for=condition=Ready", "pod/"+id+"-target", "--timeout=120s"); err != nil {
		return []finding{{"warn", "could not test NetworkPolicy enforcement, the probe Pod did not start: " + lastLineOf(o, err)}}
	}
	ip, _ := k.kubectl(ctx, "-n", namespace, "get", "pod", id+"-target", "-o", "jsonpath={.status.podIP}")
	// A policy takes a moment to reach the nodes.
	time.Sleep(5 * time.Second)
	o, _ := k.kubectl(ctx, "-n", namespace, "run", id+"-client", "--image=busybox:1.37", "--restart=Never", "--rm", "-i", "--quiet",
		"--command", "--", "sh", "-c", fmt.Sprintf("nc -w 5 %s 2375 </dev/null | grep -q ok && echo REACHED || echo blocked", strings.TrimSpace(ip)))
	switch {
	case strings.Contains(o, "blocked"):
		return []finding{{"ok", "the CNI enforces NetworkPolicies: an isolated Pod could not be reached"}}
	case strings.Contains(o, "REACHED"):
		return []finding{{"fail", "the CNI does NOT enforce NetworkPolicies: a Pod that a policy isolates was reached. " +
			"The Docker port of every sandbox would be open to every Pod in the cluster. Do not run kubedojo here until the CNI enforces policies " +
			"(k3s: do not start it with --disable-network-policy)"}}
	}
	return []finding{{"warn", "could not test NetworkPolicy enforcement: " + lastLineOf(o, fmt.Errorf("no answer from the probe"))}}
}

// doctorKubernetes checks a cluster before kubedojo is installed into it.
func doctorKubernetes(ctx context.Context, kubeContext, namespace string, sessions int, deep bool, arch string) error {
	need := clusterNeeds{sessions: sessions,
		cpu:                 env("KUBEDOJO_SANDBOX_CPU_REQUEST", "1"),
		memory:              env("KUBEDOJO_SANDBOX_MEMORY_REQUEST", "3Gi"),
		disk:                env("KUBEDOJO_SANDBOX_EPHEMERAL_REQUEST", "10Gi"),
		sandboxArchitecture: arch,
		toleration:          os.Getenv("KUBEDOJO_SANDBOX_TOLERATION"),
		selector:            map[string]string{}}
	for _, kv := range strings.Split(os.Getenv("KUBEDOJO_SANDBOX_NODE_SELECTOR"), ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(kv), "="); ok {
			need.selector[k] = v
		}
	}
	if need.sandboxArchitecture == "any" {
		need.sandboxArchitecture = ""
	}
	fmt.Printf("Checking the cluster for kubedojo (namespace %s, %d sessions):\n", namespace, sessions)
	k := kube{run: func(ctx context.Context, name string, args ...string) (string, error) {
		b, err := dockerhost.Local().Command(ctx, name, args...).CombinedOutput()
		return strings.TrimSpace(string(b)), err
	}, context: kubeContext}
	return report(checkCluster(ctx, k, namespace, need, deep))
}

// doctorHosted proves, from inside the server's Pod, that a sandbox works:
// the Pod starts, its Docker answers, a cluster is created in it. It uses the
// server's own configuration and removes what it created.
func doctorHosted(ctx context.Context, drv *sandbox.Kind, cluster bool) error {
	fmt.Println("Starting a probe sandbox with this server's configuration:")
	var findings []finding
	add := func(level, format string, a ...any) {
		findings = append(findings, finding{level, fmt.Sprintf(format, a...)})
	}
	hosts, err := hostProvider(ctx, "kubernetes", discardLogger())
	if err != nil {
		add("fail", "the sandbox configuration is not valid: %v", err)
		return report(findings)
	}
	id := fmt.Sprintf("%s%06d", k8shost.ProbePrefix, rand.IntN(1000000))
	logf := func(format string, a ...any) { fmt.Printf("    %s\n", fmt.Sprintf(format, a...)) }
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := hosts.Release(c, id); err != nil {
			fmt.Println("  ! could not remove the probe Pod kd-sandbox-"+id+":", err)
		}
	}()

	start := time.Now()
	host, err := hosts.Acquire(ctx, id, logf)
	if err != nil {
		add("fail", "the sandbox Pod did not start: %v", err)
		return report(findings)
	}
	add("ok", "the sandbox Pod is ready after %s", time.Since(start).Round(time.Second))

	if o, err := host.Command(ctx, "docker", "info", "--format", "{{.ServerVersion}} cgroup {{.CgroupVersion}}").CombinedOutput(); err != nil {
		add("fail", "the server cannot reach the sandbox's Docker at %s: %s. A NetworkPolicy or the CNI blocks the server", host.Endpoint, strings.TrimSpace(string(o)))
		return report(findings)
	} else {
		add("ok", "Docker in the sandbox answers: %s", strings.TrimSpace(string(o)))
	}
	if !cluster {
		add("warn", "not checked: creating a cluster in the sandbox. Run with --cluster (about a minute)")
		return report(findings)
	}
	keys, err := sandbox.NewKeyPair()
	if err != nil {
		return err
	}
	start = time.Now()
	env := sandbox.Env{Alias: "probe", Type: sandbox.EnvCluster}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	err = drv.CreateEnv(cctx, host, id, env.Alias, sandbox.EnvSpec{Name: "probe"}, keys, logf)
	_ = drv.Destroy(context.WithoutCancel(ctx), host, id, []sandbox.Env{env})
	if err != nil {
		add("fail", "a cluster could not be created in the sandbox: %v", err)
	} else {
		add("ok", "a cluster was created in the sandbox in %s", time.Since(start).Round(time.Second))
	}
	return report(findings)
}
