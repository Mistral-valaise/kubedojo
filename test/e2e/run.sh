#!/usr/bin/env bash
# The end-to-end proof of hosted mode: kubedojo on a fresh single-node k3s
# (in Docker, with k3d), from the preflight to an uninstall that keeps the
# learners' history. It is what the CI job "hosted" runs and what
# docs/deployment-k3s.md asks you to repeat on a real host.
#
#   make images server-image sandbox-host go VERSION=e2e
#   SERVER_IMAGE=kubedojo/server:e2e SANDBOX_IMAGE=kubedojo/sandbox-host:e2e test/e2e/run.sh
#
#   CLUSTER       name of the k3d cluster to create (default kubedojo-e2e)
#   PORT          local port of the cluster's ingress (default 18080)
#   KEEP=1        leave the cluster running at the end, to look around
#   LGTM=0        without the evaluation observability stack (saves a 0.9 GB pull)
#   REPORT        file to write the measurements to (default test/e2e/report.txt)
#
# It needs docker, k3d, kubectl, helm, go and bin/kubedojo. It uses a
# kubeconfig of its own and does not touch your kubectl context.
set -euo pipefail

cd "$(dirname "$0")/../.."
CLUSTER=${CLUSTER:-kubedojo-e2e}
PORT=${PORT:-18080}
NS=kubedojo
HOST=kubedojo.localhost
LGTM=${LGTM:-1}
REPORT=${REPORT:-test/e2e/report.txt}
: "${SERVER_IMAGE:?set SERVER_IMAGE, e.g. kubedojo/server:e2e}"
: "${SANDBOX_IMAGE:?set SANDBOX_IMAGE, e.g. kubedojo/sandbox-host:e2e}"
ARCH=$(docker info --format '{{.Architecture}}' | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')

step() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
fail() { printf '\033[31mFAILED: %s\033[0m\n' "$*" >&2; exit 1; }

export KUBECONFIG
KUBECONFIG=$(mktemp -t kubedojo-e2e.XXXXXX)
PF=""
cleanup() {
  code=$?
  [ -n "$PF" ] && kill $PF 2>/dev/null || true
  if [ $code -ne 0 ]; then
    step "diagnostics"
    kubectl -n $NS get pods,pvc,ingress,networkpolicy -o wide 2>&1 | tail -30 || true
    kubectl -n $NS get events --sort-by=.lastTimestamp 2>&1 | tail -25 || true
    kubectl -n $NS logs deploy/kubedojo -c kubedojo --tail=40 2>&1 || true
  fi
  if [ "${KEEP:-}" != "1" ]; then
    k3d cluster delete "$CLUSTER" >/dev/null 2>&1 || true
    rm -f "$KUBECONFIG"
  else
    echo "the cluster stays: export KUBECONFIG=$KUBECONFIG"
  fi
  exit $code
}
trap cleanup EXIT

repo() { echo "${1%:*}"; }
tag() { echo "${1##*:}"; }
chart=(deploy/helm/kubedojo -n $NS -f test/e2e/values.yaml
  --set "image.repository=$(repo "$SERVER_IMAGE")" --set "image.tag=$(tag "$SERVER_IMAGE")"
  --set "sandbox.image.repository=$(repo "$SANDBOX_IMAGE")" --set "sandbox.image.tag=$(tag "$SANDBOX_IMAGE")"
  --set "lgtm.enabled=$([ "$LGTM" = 1 ] && echo true || echo false)")
api() { curl -fsS -H "Host: $HOST" "http://127.0.0.1:$PORT$1"; }
kd() { kubectl -n $NS exec deploy/kubedojo -c kubedojo -- kubedojo "$@"; }
graded() { kd backup --out "/data/e2e-$1.db" | sed -E 's/.* ([0-9]+) graded questions.*/\1/'; }

step "a fresh single-node k3s"
k3d cluster delete "$CLUSTER" >/dev/null 2>&1 || true
k3d cluster create "$CLUSTER" --servers 1 --agents 0 -p "$PORT:80@loadbalancer" \
  --kubeconfig-update-default=false --kubeconfig-switch-context=false --wait --timeout 300s >/dev/null
k3d kubeconfig get "$CLUSTER" > "$KUBECONFIG"
kubectl get nodes -o wide
k3d image import "$SERVER_IMAGE" "$SANDBOX_IMAGE" -c "$CLUSTER" >/dev/null
# k3s installs Traefik after the node is ready.
for _ in $(seq 1 60); do kubectl get ingressclass traefik >/dev/null 2>&1 && break; sleep 3; done
kubectl create namespace $NS >/dev/null
kubectl label namespace $NS pod-security.kubernetes.io/enforce=privileged >/dev/null

step "preflight: is this cluster fit for kubedojo?"
./bin/kubedojo doctor --kubernetes --namespace $NS --max-sessions 2 --sandbox-arch "$ARCH" --deep

step "helm lint, template, install --wait"
helm lint "${chart[@]}" >/dev/null
helm template kubedojo "${chart[@]}" >/dev/null
START=$(date +%s)
helm install kubedojo "${chart[@]}" --wait --timeout 10m >/dev/null
echo "installed in $(( $(date +%s) - START ))s"
kubectl -n $NS get pods
for _ in $(seq 1 30); do api /api/ready >/dev/null 2>&1 && break; sleep 2; done
api /api/ready >/dev/null || fail "kubedojo does not answer through the ingress"

step "probe: a sandbox with a cluster, from the server's Pod"
kd doctor --hosted --cluster

step "a learner's sessions through the ingress: a Linux exercise, a cluster, a timed exam"
KUBEDOJO_E2E_URL="http://127.0.0.1:$PORT" KUBEDOJO_E2E_HOST=$HOST \
  go test -count=1 -timeout 40m -v ./test/e2e/ 2>&1 | grep -v '^=== ' | tee /tmp/kubedojo-e2e-tests.log
grep -q '^ok' /tmp/kubedojo-e2e-tests.log || fail "the sessions did not work"

step "two sandboxes at once, and what they use"
sids=()
for track in cka ckad; do
  sid=$(curl -fsS -H "Host: $HOST" -X POST "http://127.0.0.1:$PORT/api/sessions" -H 'Content-Type: application/json' \
    -d "{\"track\":\"$track\",\"mode\":\"practice\"}" | sed -E 's/.*"id":"([^"]+)".*/\1/')
  q=$(api "/api/tracks/$track" | sed -E 's/.*"questions":\[\{"id":"([^"]+)".*/\1/')
  curl -fsS -o /dev/null -H "Host: $HOST" -X POST "http://127.0.0.1:$PORT/api/sessions/$sid/questions/$q/prepare"
  sids+=("$sid")
done
for sid in "${sids[@]}"; do
  for _ in $(seq 1 120); do
    api "/api/sessions/$sid" | grep -q '"status":"ready"' && break
    api "/api/sessions/$sid" | grep -q '"status":"error"' && fail "session $sid failed"
    sleep 5
  done
  api "/api/sessions/$sid" | grep -q '"status":"ready"' || fail "session $sid was not ready in ten minutes"
done
# Let the clusters settle: they start with all components at once, and
# metrics-server reports the average of the last window.
sleep 150
{
  echo "kubedojo hosted e2e, $(date -u +%Y-%m-%dT%H:%MZ), $ARCH, $(kubectl get nodes -o jsonpath='{.items[0].status.nodeInfo.kubeletVersion}')"
  echo "node allocatable: $(kubectl get nodes -o jsonpath='{.items[0].status.allocatable.cpu} CPU, {.items[0].status.allocatable.memory} memory')"
  echo "two sandboxes, each with one idle two-node cluster:"
  kubectl -n $NS top pods --no-headers | sed 's/^/  /'
  echo "node:"
  kubectl top nodes --no-headers | sed 's/^/  /'
} | tee "$REPORT"
# The limits must cover the clusters: the nodes of a cluster live below the Pod's cgroup.
pod=$(kubectl -n $NS get pod -l app.kubernetes.io/component=sandbox -o name | head -1)
kubectl -n $NS exec "$pod" -- sh -c 'c=$(docker ps -q | head -1); cat /proc/$(docker inspect -f "{{.State.Pid}}" $c)/cgroup' | grep -q kubepods \
  || fail "the containers of a sandbox are outside the cgroup of its Pod: its limits do not apply to them"
echo "the containers of a sandbox are inside the cgroup of its Pod"

step "the Docker port of a sandbox: reachable for the server only"
ip=$(kubectl -n $NS get "$pod" -o jsonpath='{.status.podIP}')
got=$(kubectl -n default run e2e-intruder --image=busybox:1.37 --restart=Never --rm -i --quiet --command -- \
  sh -c "nc -z -w 5 $ip 2375 && echo REACHED || echo blocked" 2>/dev/null | tail -1)
[ "$got" = blocked ] || fail "a Pod of another namespace reached the Docker port of a sandbox ($got)"
kubectl -n $NS exec deploy/kubedojo -c kubedojo -- sh -c "nc -z -w 5 $ip 2375" || fail "the server cannot reach the sandbox"
echo "blocked for a foreign Pod, open for the server"

if [ "$LGTM" = 1 ]; then
  step "observability: traces, logs and metrics, read back through Grafana"
  kubectl -n $NS port-forward svc/kubedojo-lgtm 13000:3000 14318:4318 >/dev/null 2>&1 &
  PF=$!
  sleep 5
  KUBEDOJO_VERIFY_CHART=1 KUBEDOJO_VERIFY_GRAFANA_PORT=13000 KUBEDOJO_VERIFY_OTLP_PORT=14318 \
    go test -count=1 -run VerifyRunningStack -v ./internal/observability/ 2>&1 | grep -E '^\s+smoke|^(ok|FAIL|---)' | tee /tmp/kubedojo-e2e-obs.log
  grep -q '^ok' /tmp/kubedojo-e2e-obs.log || fail "the observability stack does not work"
  # kubedojo's own telemetry, from the sessions above.
  curl -fsS "http://127.0.0.1:13000/api/datasources/proxy/uid/prometheus/api/v1/query" \
    --data-urlencode 'query=sum(kubedojo_sandbox_provision_duration_seconds_count)' | grep -q '"value"' \
    || fail "no kubedojo metric arrived in Prometheus"
  echo "kubedojo's own metrics arrived"
  kill $PF; PF=""
fi

step "backup, upgrade, rollback"
before=$(graded before-upgrade)
echo "graded questions in the history: $before"
[ "$before" -ge 1 ] || fail "the history is empty after the sessions above"
helm upgrade kubedojo "${chart[@]}" --reset-then-reuse-values --set maxTerminals=6 --wait --timeout 5m >/dev/null
kubectl -n $NS get deploy kubedojo -o jsonpath='{.spec.template.spec.containers[0].args}' | grep -q -- '--max-terminals=6' || fail "the upgrade did not arrive"
# Sandboxes survive the restart of the server.
[ "$(kubectl -n $NS get pod -l app.kubernetes.io/component=sandbox --no-headers | grep -c Running)" -ge 2 ] || fail "sandboxes did not survive the upgrade"
helm rollback kubedojo -n $NS --wait --timeout 5m >/dev/null
kubectl -n $NS get deploy kubedojo -o jsonpath='{.spec.template.spec.containers[0].args}' | grep -q -- '--max-terminals=4' || fail "the rollback did not arrive"
for _ in $(seq 1 30); do api /api/ready >/dev/null 2>&1 && break; sleep 2; done
[ "$(graded after-rollback)" -ge "$before" ] || fail "the history shrank over upgrade and rollback"
echo "upgraded and rolled back; sandboxes and history survived"

step "uninstall: no sandbox is left, the history is"
helm uninstall kubedojo -n $NS --wait --timeout 5m >/dev/null
for _ in $(seq 1 60); do
  [ -z "$(kubectl -n $NS get pod -l app.kubernetes.io/component=sandbox -o name)" ] && break; sleep 3
done
left=$(kubectl -n $NS get pod -l app.kubernetes.io/component=sandbox -o name)
[ -z "$left" ] || fail "sandbox Pods survived the uninstall: $left"
kubectl -n $NS get pvc kubedojo >/dev/null || fail "the volume with the history was deleted"
echo "no sandbox Pod left; the volume with the history is still there"

step "install again: the history is back"
helm install kubedojo "${chart[@]}" --wait --timeout 10m >/dev/null
for _ in $(seq 1 30); do api /api/ready >/dev/null 2>&1 && break; sleep 2; done
after=$(graded after-reinstall)
[ "$after" -ge "$before" ] || fail "the history has $after graded questions after the reinstall, $before before"
echo "graded questions in the history: $after"

step "all of it worked"
cat "$REPORT"
