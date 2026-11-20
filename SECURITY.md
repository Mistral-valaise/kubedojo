# Security

## Reporting a vulnerability

Please report security problems privately through GitHub's
[private vulnerability reporting](https://github.com/Mistral-valaise/kubedojo/security/advisories/new),
not in a public issue. Include what you found, how to reproduce it and which version you used.
You can expect a first answer within a week.

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
