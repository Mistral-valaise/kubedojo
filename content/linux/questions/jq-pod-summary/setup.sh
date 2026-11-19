#!/usr/bin/env bash
# The saved output of `kubectl get pods -A -o json`, built from a table so
# that every number is known. Traps: Pods with two containers (restarts must
# be added up), Pods without containerStatuses, an init container image, and
# a finished Job Pod that is not a problem.
set -euo pipefail

base=/opt/course/jq-pod-summary
rm -rf "$base"
mkdir -p "$base"

# namespace name phase node container=image=restarts[,...] [init image]
# restarts "-" means: the container has no status yet.
jq -Rn '
  def pod:
    split(" ") as [$ns, $name, $phase, $node, $containers, $init]
    | ($containers | split(",") | map(split("=") | {name: .[0], image: .[1], restarts: .[2]})) as $cs
    | {
        apiVersion: "v1",
        kind: "Pod",
        metadata: {name: $name, namespace: $ns, labels: {"app.kubernetes.io/name": ($name | split("-")[0])}},
        spec: (
          (if $init then {initContainers: [{name: "init", image: $init}]} else {} end)
          + {containers: ($cs | map({name, image})), nodeName: (if $node == "-" then null else $node end)}
          | with_entries(select(.value != null))
        ),
        status: (
          {phase: $phase}
          + (if ($cs | all(.restarts == "-")) then {} else {
              containerStatuses: ($cs | map({name, image, ready: ($phase == "Running"), restartCount: (.restarts | tonumber)}))
            } end)
        )
      };
  {apiVersion: "v1", kind: "List", metadata: {resourceVersion: ""}, items: [inputs | select(length > 0) | pod]}
' > "$base/pods.json" <<'TABLE'
shop web-7d9f6c5b8-4xkpl Running node-a nginx=nginx:1.27-alpine=0,proxy=envoyproxy/envoy:v1.31.0=2
shop web-7d9f6c5b8-9hq2m Running node-b nginx=nginx:1.27-alpine=1,proxy=envoyproxy/envoy:v1.31.0=0
shop api-5c6b7d8f9-k2j4n Running node-a api=registry.shop.internal/api:2.4.1=7,proxy=envoyproxy/envoy:v1.31.0=1
shop api-5c6b7d8f9-zt8vw Running node-c api=registry.shop.internal/api:2.4.1=20,proxy=envoyproxy/envoy:v1.31.0=3
shop migrate-2-4-1-q8d7s Succeeded node-b migrate=registry.shop.internal/migrate:2.4.1=0
shop cache-0 Pending - redis=redis:7.4-alpine=-
payments ledger-0 Running node-c postgres=postgres:16.4=0
payments gateway-66f7b9c4d-aa7lp Failed node-a gateway=registry.shop.internal/gateway:1.9.0=4
payments gateway-66f7b9c4d-bb3rx Running node-b gateway=registry.shop.internal/gateway:1.9.0=0,proxy=envoyproxy/envoy:v1.31.0=5
kube-system coredns-6f6b679f8f-m5x9c Running node-a coredns=registry.k8s.io/coredns/coredns:v1.11.3=0
monitoring prometheus-0 Running node-c prometheus=prom/prometheus:v2.54.1=3 busybox:1.36
monitoring loki-0 Unknown node-c loki=grafana/loki:3.1.1=0
TABLE

sha256sum "$base/pods.json" | cut -d' ' -f1 > "$base/.manifest"

# The learner writes the reports into the directory; the snapshot is root's.
chown candidate:candidate "$base"
