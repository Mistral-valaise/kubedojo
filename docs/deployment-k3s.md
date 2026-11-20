# kubedojo on a single k3s host

A guide to copy and paste: from an empty Linux machine to a learner solving a Kubernetes exercise in
the browser, with traces, logs and metrics in Grafana, and back to an empty machine. For what the
chart can do beyond this, see [deployment.md](deployment.md).

**Who this is for.** A private pilot: you and people you trust, on a mini-PC at home or a rented VM.
Every session gets a root shell in a **privileged** Pod. Anyone who can open the URL can become root
on the host. A login and a NetworkPolicy do not change that; they decide who can open the URL. Do not
offer this to strangers. That needs sandboxes in virtual machines, which kubedojo does not have.

## What was tested

| Target | Status |
|---|---|
| Single-node **k3s v1.35**, Traefik, local-path storage, in Docker (k3d) on **amd64** | every pull request that touches the chart, and nightly: [`test/e2e/run.sh`](../test/e2e/run.sh) |
| The same on **arm64** (Apple Silicon, with a sandbox host image built locally) | by hand, the same script |
| Single-node k3s on a **real amd64 host** | this guide; record your run in the table at the end |
| Other Kubernetes with a CNI that enforces NetworkPolicies and a default StorageClass | expected to work, **not tested**. `kubedojo doctor --kubernetes --deep` tells you what is missing |
| ingress-nginx instead of Traefik | not tested; needs two annotations, see [deployment.md](deployment.md#ingress) |
| OpenShift | **not supported** as it comes: sandbox Pods need the `privileged` SCC, which a cluster administrator must grant to kubedojo's ServiceAccount. Nobody has tried it |
| arm64 nodes with the published images | not supported: the published sandbox host image is amd64 only. Build your own with `make sandbox-host` |
| cgroup v1 nodes | sandboxes start, but their limits do not cover the clusters inside. Use cgroup v2 |

"Tested" means the script ran green: preflight, install, a sandbox with a cluster, a learner's
sessions through the ingress, two sandboxes at once, the observability stack, backup, upgrade,
rollback, uninstall and reinstall. TLS, a real identity provider and a full two-hour exam are **not**
part of the automated run.

## 1. The host

| | Minimum for 2 sessions | Comfortable for 5 |
|---|---|---|
| CPU | 4 cores, amd64 | 8 cores |
| Memory | 16 GB | 32 GB |
| Disk | 80 GB free, SSD | 200 GB |
| OS | a current Linux with cgroup v2 (Ubuntu 22.04 or later, Debian 12 or later) | |

The sizing is explained in [deployment.md](deployment.md#how-many-sessions-fit-on-one-node); the
numbers there are estimates until you have measured your host (step 7).

```bash
# cgroup v2? This must print cgroup2fs.
stat -fc %T /sys/fs/cgroup

# A cluster inside a sandbox inside k3s needs more inotify instances than the default.
sudo tee /etc/sysctl.d/99-kubedojo.conf <<'EOF'
fs.inotify.max_user_instances = 1024
fs.inotify.max_user_watches = 1048576
EOF
sudo sysctl --system

# k3s, with its defaults: Traefik, local-path storage, NetworkPolicies enforced.
curl -sfL https://get.k3s.io | INSTALL_K3S_CHANNEL=v1.35 sh -
sudo k3s kubectl get nodes

# kubectl and helm for your user.
mkdir -p ~/.kube && sudo cat /etc/rancher/k3s/k3s.yaml > ~/.kube/config && chmod 600 ~/.kube/config
curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-4 | bash
```

Do not start k3s with `--disable-network-policy`: the Docker daemon of every sandbox would be open to
every Pod.

## 2. Preflight

Get the kubedojo binary of the release you install (the
[release page](https://github.com/Mistral-valaise/kubedojo/releases) has `linux_amd64`), then:

```bash
kubectl create namespace kubedojo
kubectl label namespace kubedojo pod-security.kubernetes.io/enforce=privileged
kubedojo doctor --kubernetes --max-sessions 2 --deep
```

Every line should start with `✓`. A `✗` says what to change; fix it before you go on. A `!` about a
public node address prints the `extraExceptCIDRs` you need in step 3.

## 3. Values

Pick the example that fits and edit the lines marked `CHANGE`:

- [`examples/k3s-private.yaml`](../deploy/helm/kubedojo/examples/k3s-private.yaml): a machine in
  your own network, reachable over a VPN or the LAN only, no login.
- [`examples/k3s-public.yaml`](../deploy/helm/kubedojo/examples/k3s-public.yaml): a VM with a public
  address, HTTPS from Let's Encrypt and an OIDC login.

For the public one you need, before the install:

```bash
# DNS: an A record for your host name that points at the VM.

# cert-manager, for the certificate.
helm install cert-manager oci://quay.io/jetstack/charts/cert-manager --version v1.21.3 \
  -n cert-manager --create-namespace --set crds.enabled=true
kubectl apply -f - <<'EOF'
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata: {name: letsencrypt}
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: you@example.com            # CHANGE
    privateKeySecretRef: {name: letsencrypt-account}
    solvers: [{http01: {ingress: {ingressClassName: traefik}}}]
EOF

# The OIDC client: register https://<your host>/oauth2/callback with your identity provider.
kubectl -n kubedojo create secret generic kubedojo-oidc \
  --from-literal=client-secret='...' \
  --from-literal=cookie-secret="$(head -c 32 /dev/urandom | base64)"
```

## 4. Install

```bash
helm install kubedojo oci://ghcr.io/mistral-valaise/charts/kubedojo --version <release> \
  -n kubedojo -f my-values.yaml --wait --timeout 10m
kubectl -n kubedojo get pods
```

The first sandbox pulls the sandbox host image, about 2 GB. Pull it ahead of time if you want the
first session to start fast:

```bash
sudo k3s crictl pull ghcr.io/mistral-valaise/kubedojo-sandbox-host:v<release>
```

## 5. Verify

```bash
# A sandbox, its Docker, and a cluster inside it. About a minute.
kubectl -n kubedojo exec deploy/kubedojo -c kubedojo -- kubedojo doctor --hosted --cluster
```

Then, as a learner:

1. Open the URL. With the public example you are sent to your identity provider first.
2. **Linux & Vim Essentials → any exercise.** The terminal appears within a minute; the first time
   includes pulling the sandbox host image.
3. Solve it or not, press **Check**: you get points and an explanation per check.
4. Leave the terminal alone for two minutes, then type again: it still answers.
5. **CKA → any question.** A cluster with two nodes is created; `kubectl get nodes` in the terminal
   shows them `Ready`.
6. Start a short **exam** on the Linux track, finish it, read the report.
7. Close the tab, open the URL again: the sessions are still there.

The same as a script, from a machine that can reach the URL (it does not go through a login, so use
it with the private example or a port-forward):

```bash
git clone https://github.com/Mistral-valaise/kubedojo && cd kubedojo
KUBEDOJO_E2E_URL=http://<your host> go test -count=1 -timeout 40m -v ./test/e2e/
```

## 6. Observability

With `lgtm.enabled: true`, as in both examples:

```bash
kubectl -n kubedojo port-forward svc/kubedojo-lgtm 3000
```

Open <http://localhost:3000/dashboards?tag=kubedojo>. After the sessions of step 5:

- *Platform health* shows the requests, and *Sessions & sandboxes* the sandbox starts.
- Click a dot in *Time to a ready sandbox*: that is one sandbox start. Follow it to its trace, and
  from a span to its log lines.
- *Alerting → Alert rules* lists seven rules in the folder *kubedojo*, all *Normal*.

How to read all of this is in
[observability.md](observability.md#a-first-investigation). The stack in the cluster is for
evaluation: one Pod, no redundancy.

## 7. Measure

While two or more people work, or after starting several sessions yourself:

```bash
kubectl -n kubedojo top pods            # per sandbox: what its clusters really use
kubectl top nodes
free -h                                  # on the host
df -h /var/lib/rancher                   # k3s keeps images and Pod storage here
sudo k3s crictl stats --no-trunc | head
```

Compare with what you reserved: `maxSessions × memoryLimit` should stay below the memory of the
host minus about 3 GB. If `kubectl top pods` shows sandboxes near their memory limit during an exam,
raise `sandbox.resources.memoryLimit` and lower `maxSessions`.

## 8. Day two

```bash
# Backup of the history (works while kubedojo runs).
kubectl -n kubedojo exec deploy/kubedojo -c kubedojo -- kubedojo backup --out /data/backup.db
kubectl -n kubedojo exec deploy/kubedojo -c kubedojo -- cat /data/backup.db > kubedojo-backup.db

# Upgrade, and back.
helm upgrade kubedojo oci://ghcr.io/mistral-valaise/charts/kubedojo --version <new release> \
  -n kubedojo --reset-then-reuse-values --wait
helm rollback kubedojo -n kubedojo

# Sandboxes that are running.
kubectl -n kubedojo get pods -l app.kubernetes.io/component=sandbox
```

| Symptom | Look at |
|---|---|
| A session stays at "Starting the sandbox host" | The session's progress log says why: no room on the node, the image cannot be pulled. `kubectl -n kubedojo describe pod -l app.kubernetes.io/component=sandbox` |
| "the sandbox of this session no longer exists" | The Pod was evicted or deleted. `kubectl -n kubedojo get events --sort-by=.lastTimestamp`. End the session, start a new one |
| Sandboxes are evicted | The node is out of disk or memory. `kubectl describe node`, then fewer sessions or smaller limits |
| The disk fills up | `sudo k3s crictl rmi --prune` removes unused images; the image cache has its own volume (`registryCache.persistence.size`) |
| The terminal disconnects after a minute | A proxy in front of Traefik with a short idle timeout. Traefik itself keeps the connection |
| Login loops forever | `oidc.cookieSecure` is on and the site is reached over plain HTTP |

Restore, the upgrade notes and more are in [deployment.md](deployment.md#operations).

## 9. Remove it

```bash
helm uninstall kubedojo -n kubedojo          # sandboxes go with it; the history stays
kubectl -n kubedojo delete pvc --all         # the history and the caches, for good
kubectl delete namespace kubedojo
/usr/local/bin/k3s-uninstall.sh              # k3s itself
sudo rm /etc/sysctl.d/99-kubedojo.conf
```

## Your run

The automated run measures a small test cluster. A real host is what the sizing table should be
based on. Fill this in after step 7 and, if you like, send it as an issue or a pull request:

| | |
|---|---|
| Date, kubedojo release | |
| Host: CPU, memory, disk | |
| OS, kernel, k3s version | |
| `maxSessions`, sandbox requests and limits | |
| Time from `helm install` to ready | |
| First session: time to a ready terminal | |
| Later sessions: time to a ready terminal | |
| Sessions at the same time | |
| `kubectl top pods`: memory per sandbox, practice | |
| `kubectl top pods`: memory per sandbox, exam | |
| `free -h` on the host at the peak | |
| Disk used under `/var/lib/rancher` | |
| What did not work | |
