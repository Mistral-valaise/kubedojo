# Security

## Reporting a vulnerability

Please report security problems privately through GitHub's
[private vulnerability reporting](https://github.com/Mistral-valaise/kubedojo/security/advisories/new),
not in a public issue. Include what you found, how to reproduce it and which version you used.
You can expect a first answer within a week.

## Verifying a release

Releases newer than v0.4.0 are signed with [Sigstore](https://www.sigstore.dev/) by the workflow
that builds them. There is no key to trust: every signature carries a short-lived certificate that
names the workflow and the tag it ran for, and is recorded in a public transparency log. The
commands below need [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) 3 or
newer and take `v1.2.3` as the release.

**Binaries.** `checksums.txt` lists every archive and its SBOM (`<archive>.sbom.json`, SPDX), and
`checksums.txt.sigstore.json` is the signature of that list. Download both next to the archive:

```sh
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github\.com/Mistral-valaise/kubedojo/\.github/workflows/release\.yaml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --check --ignore-missing checksums.txt   # macOS: shasum -a 256 --check --ignore-missing
```

**Images.** The same check for `kubedojo-server:v1.2.3`, `kubedojo-sandbox-host:v1.2.3`,
`kubedojo-node:<kubernetes version>-v1.2.3`, `kubedojo-terminal:<kubernetes version>-v1.2.3` and
`kubedojo-linux:trixie-v1.2.3`:

```sh
cosign verify ghcr.io/mistral-valaise/kubedojo-server:v1.2.3 \
  --certificate-identity-regexp '^https://github\.com/Mistral-valaise/kubedojo/\.github/workflows/release\.yaml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

All but the sandbox host also carry an SBOM and build provenance as attestations:
`docker buildx imagetools inspect <image> --format '{{ json .SBOM }}'` (or `.Provenance`).

**Helm chart.** The chart's version has no `v`:

```sh
cosign verify ghcr.io/mistral-valaise/charts/kubedojo:1.2.3 \
  --certificate-identity-regexp '^https://github\.com/Mistral-valaise/kubedojo/\.github/workflows/release\.yaml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

To accept one release only, replace the regular expression by `--certificate-identity` with the
full value, which ends in `release.yaml@refs/tags/v1.2.3`.

## What to keep in mind when running kubedojo

kubedojo gives its users a root shell on real Kubernetes nodes. That is the product, and it shapes the
threat model:

- **Local mode** binds to `127.0.0.1` and runs sandboxes as privileged containers on your Docker daemon.
  Treat everything inside a sandbox as able to reach your machine's Docker host. Do not expose the port
  to a network.
- **Hosted mode** is single-tenant: for one team whose members trust each other. Sandbox Pods are
  privileged; a user should be assumed able to become root on the node that runs their sandbox. Use a
  dedicated, tainted node pool without cloud credentials, keep the NetworkPolicy enabled and put
  authentication in front. See [docs/deployment.md](docs/deployment.md).
- kubedojo is **not** a hardened multi-tenant service. Do not offer it to untrusted users without
  VM-level isolation. A login (OIDC) and the NetworkPolicy are not that isolation: they decide who
  gets a sandbox and what a sandbox can reach over the network, not what a user can do to the node
  from inside one.

What hosted mode relies on, and where that ends:

| Protection | What it does | What it does not do |
|---|---|---|
| OIDC login | only people your identity provider admits get sessions; each has their own | limit what an admitted user can do |
| NetworkPolicy of the sandboxes | only the server reaches a sandbox's Docker daemon; sandboxes do not reach private addresses | protect when the CNI does not enforce policies, against processes with host networking on the node, or for IPv6. A node's public address must be listed by hand (`extraExceptCIDRs`) |
| Docker daemon of a sandbox | listens on TCP 2375 **without authentication or TLS** | protect itself: whoever reaches the port is root in the sandbox and, through it, on the node. Mutual TLS is planned |
| Resource limits of a sandbox | CPU, memory and disk of one session are bounded (cgroup v2 nodes) | stop a user who is root on the node from leaving the cgroup |
| ServiceAccount of the server | Pods in its own Namespace, and reading its own Deployment | contain a compromised server: creating Pods in a Namespace that admits privileged ones is root on the nodes |
| `/lib/modules` | mounted read-only from the node into every sandbox | hide the node's kernel modules from a session |

`kubedojo doctor --kubernetes --deep` checks before an installation that privileged Pods are
admitted where they should be and that NetworkPolicies are enforced.

Reports that amount to "a user can break out of a sandbox onto the sandbox node" describe this known
limit. Reports about reaching *other users' sandboxes*, the kubedojo server, its database, or the
cluster's API from a sandbox are very welcome.

### The API has no login of its own

Locally the API is protected by where it listens, in hosted mode by the proxy in front of it. A web
page you visit while kubedojo runs is on your machine too, so the server also checks what a browser
tells it:

| Protection | What it does | What it does not do |
|---|---|---|
| Listening on `127.0.0.1` | other machines do not reach a local kubedojo | keep out other users and programs of the same machine |
| Host allowlist | a request is answered only under a name the server is known by: `localhost`, `127.0.0.1`, `[::1]`, names below `.localhost`, the address it listens on, and what `--allowed-hosts` (`KUBEDOJO_ALLOWED_HOSTS`, in the chart `allowedHosts`) adds. Other names get `403`. This stops DNS rebinding, where a page's own domain is made to resolve to `127.0.0.1` | apply to `/api/health` and `/api/ready`; apply at all to a server that listens on every interface without `--allowed-hosts` (it warns at start) |
| Cross-origin protection | requests that change something (`POST`, `PATCH`, `DELETE`) are rejected with `403` when the browser marks them as coming from another site; the terminal's WebSocket checks its `Origin` | stop a program that is not a browser |
| JSON bodies only | a request body is read only when it is sent with `Content-Type: application/json`, which a page of another site cannot send without the browser asking the server first. Anything else gets `415` | |
| Content-Security-Policy and related headers | the UI loads scripts, styles, images and fonts from the server only and connects only to it; it cannot be framed | allow-list inline styles, which React and xterm.js need |
| File modes | the data directory is `0700`, the database, its backups and the settings are `0600` | protect a volume in Kubernetes, where access is the cluster's matter; change `<data>/observability`, which Grafana reads |
| Session IDs | 12 random characters from the system's random source: the ID of a session cannot be guessed | replace a login: whoever is given an ID has that session |

Two of these change what worked before 0.5:

- A hosted kubedojo that is reached under a name the chart does not know (a second DNS name, the
  address of a NodePort or LoadBalancer Service) answers `403` until that name is in `allowedHosts`.
- A request with a body but without `Content-Type: application/json` is answered with `415`. The UI
  always sent the header; scripts that post with `curl -d` need `-H 'Content-Type: application/json'`.
