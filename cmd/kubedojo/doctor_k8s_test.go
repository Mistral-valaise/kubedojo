package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func nodes(t *testing.T, js string) nodeList {
	t.Helper()
	var n nodeList
	if err := json.Unmarshal([]byte(js), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

const oneNode = `{"items":[{"metadata":{"name":"mini","labels":{"pool":"sandbox"}},
 "status":{"allocatable":{"cpu":"8","memory":"32Gi","ephemeral-storage":"200Gi"},
  "nodeInfo":{"architecture":"amd64","kernelVersion":"6.8.0","containerRuntimeVersion":"containerd://2.0","kubeletVersion":"v1.35.5+k3s1"},
  "addresses":[{"type":"InternalIP","address":"192.168.1.20"}]}}]}`

func texts(fs []finding, level string) string {
	var out []string
	for _, f := range fs {
		if f.level == level {
			out = append(out, f.text)
		}
	}
	return strings.Join(out, "\n")
}

var defaults = clusterNeeds{sessions: 5, cpu: "1", memory: "3Gi", disk: "10Gi", sandboxArchitecture: "amd64"}

func TestNodesWithRoom(t *testing.T) {
	fs := checkNodes(nodes(t, oneNode), podList{}, defaults)
	if fail := texts(fs, "fail"); fail != "" {
		t.Fatalf("a 32 Gi node fails for 5 sessions:\n%s", fail)
	}
	// 8 CPU / 1, 32 Gi / 3 Gi, 200 Gi / 10 Gi: CPU allows 8, memory 10, disk 20.
	if ok := texts(fs, "ok"); !strings.Contains(ok, "room for 8 sandbox(es)") || !strings.Contains(ok, "CPU runs out first") {
		t.Errorf("the capacity is not explained:\n%s", ok)
	}
	if warn := texts(fs, "warn"); warn != "" {
		t.Errorf("unexpected warnings:\n%s", warn)
	}
}

// What other Pods already reserve is not free for sandboxes.
func TestNodesCountWhatIsAlreadyRequested(t *testing.T) {
	var pods podList
	_ = json.Unmarshal([]byte(`{"items":[
	 {"spec":{"nodeName":"mini","containers":[{"resources":{"requests":{"memory":"20Gi","cpu":"500m"}}}]},"status":{"phase":"Running"}},
	 {"spec":{"nodeName":"mini","containers":[{"resources":{"requests":{"memory":"64Gi"}}}]},"status":{"phase":"Succeeded"}}]}`), &pods)
	fs := checkNodes(nodes(t, oneNode), pods, defaults)
	// 12 Gi are left: 4 sandboxes of 3 Gi. The finished Pod does not count.
	fail := texts(fs, "fail")
	if !strings.Contains(fail, "room for 4 sandbox(es), maxSessions is 5") {
		t.Fatalf("want a failure that names 4 and 5, got:\n%s\n%s", fail, texts(fs, "ok"))
	}
	for _, want := range []string{"3Gi memory", "Lower maxSessions"} {
		if !strings.Contains(fail, want) {
			t.Errorf("the failure does not say %q: %s", want, fail)
		}
	}
}

func TestNodesThatCannotTakeSandboxes(t *testing.T) {
	// The published sandbox host image is amd64.
	arm := strings.Replace(oneNode, `"amd64"`, `"arm64"`, 1)
	if fail := texts(checkNodes(nodes(t, arm), podList{}, defaults), "fail"); !strings.Contains(fail, "arm64") || !strings.Contains(fail, "make sandbox-host") {
		t.Errorf("an arm64 node passes, or the failure does not say what to do: %s", fail)
	}
	any := defaults
	any.sandboxArchitecture = ""
	if fail := texts(checkNodes(nodes(t, arm), podList{}, any), "fail"); fail != "" {
		t.Errorf("an arm64 node fails although any architecture was allowed: %s", fail)
	}
	// A node selector that matches nothing.
	sel := defaults
	sel.selector = map[string]string{"pool": "elsewhere"}
	if fail := texts(checkNodes(nodes(t, oneNode), podList{}, sel), "fail"); !strings.Contains(fail, "no node can take sandboxes") {
		t.Errorf("no node matches the selector, and nothing fails: %s", fail)
	}
	// A taint the sandboxes do not tolerate, and the same taint tolerated.
	tainted := strings.Replace(oneNode, `"status":{"allocatable"`, `"spec":{"taints":[{"key":"kubedojo.dev/sandbox","effect":"NoSchedule"}]},"status":{"allocatable"`, 1)
	if fail := texts(checkNodes(nodes(t, tainted), podList{}, defaults), "fail"); !strings.Contains(fail, "no node can take sandboxes") {
		t.Errorf("a tainted node counts without a toleration: %s", fail)
	}
	tol := defaults
	tol.toleration = "kubedojo.dev/sandbox"
	if fail := texts(checkNodes(nodes(t, tainted), podList{}, tol), "fail"); fail != "" {
		t.Errorf("a tainted node fails although the taint is tolerated: %s", fail)
	}
}

// The NetworkPolicy keeps sandboxes away from private addresses. A node with
// a public one needs to be listed.
func TestNodesWithPublicAddresses(t *testing.T) {
	vps := strings.Replace(oneNode, `"192.168.1.20"`, `"203.0.113.10"`, 1)
	warn := texts(checkNodes(nodes(t, vps), podList{}, defaults), "warn")
	if !strings.Contains(warn, "extraExceptCIDRs: [203.0.113.10/32]") {
		t.Errorf("a public node address is not reported with the value to set: %s", warn)
	}
	if warn := texts(checkNodes(nodes(t, oneNode), podList{}, defaults), "warn"); strings.Contains(warn, "extraExceptCIDRs") {
		t.Errorf("a private node address is reported: %s", warn)
	}
}
