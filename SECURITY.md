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
  VM-level isolation.

Reports that amount to "a user can break out of a sandbox onto the sandbox node" describe this known
limit. Reports about reaching *other users' sandboxes*, the kubedojo server, its database, or the
cluster's API from a sandbox are very welcome.
