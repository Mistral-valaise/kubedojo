# Platform Security

**16% of the exam.** Everything around the workloads that makes a cluster a trustworthy platform:
where images come from, who signs what, what you can observe, how connections are protected.

## The 20% that solves 80%

### Supply chain and image repositories

The supply chain is the path from source code to a running container: source, dependencies, build,
registry, admission, runtime. Each stage has its own control:

| Stage | Risk | Control |
|---|---|---|
| Dependencies and base images | known vulnerabilities, malicious packages | minimal base images, SBOM, vulnerability scanning |
| Build | tampered build, unknown origin | isolated build platform, provenance attestation |
| Registry | image replaced, anyone can push or pull | private registry, access control, signing |
| Deployment | unreviewed image gets into the cluster | admission policy: allowed registries, digests, verified signatures |
| Runtime | new vulnerabilities, drift | regular rescans, runtime detection |

- A **tag** is a pointer that can be moved to other content; a **digest** (`image@sha256:...`) is a
  hash of the content and cannot change. Pinning by digest fixes what runs.
- A **signature** binds an identity to a digest. Sigstore is the common tooling: Cosign signs and
  verifies, Fulcio issues short-lived certificates for an OIDC identity, Rekor is the transparency
  log that records signing events. A signature proves who signed, not that the image is free of flaws.
- **`imagePullSecrets`** let the kubelet authenticate to a private registry. They do not verify
  images.

### Admission control

```
request -> authentication -> authorization -> mutating admission -> validating admission -> etcd
```

- Admission sees requests that **create, modify or delete** objects, and custom verbs such as
  connecting to a Pod. Reads (`get`, `list`, `watch`) never pass through it.
- Mutating plugins run first and may change the object; validating plugins run afterwards and can
  only accept or reject. One rejection in either phase rejects the request.

| Mechanism | Where the logic runs | Typical use |
|---|---|---|
| Built-in plugins (NodeRestriction, PodSecurity, ...) | in the API server | defaults and standard restrictions |
| ValidatingAdmissionPolicy | in the API server, CEL expressions | simple validation without a webhook |
| Validating / mutating webhooks | a service you operate | own logic, policy engines |
| Kyverno | webhook, policies as Kubernetes resources | validate, mutate, generate, verify images |
| OPA Gatekeeper | webhook, ConstraintTemplates and Constraints | validate, mutate, audit existing resources |

Whoever may edit webhook configurations can read every object that is admitted, and with a
mutating webhook also change it.

### Observability

| Signal | Source | Answers |
|---|---|---|
| Audit log | API server | who did what to which object through the API, and when |
| Runtime events | kernel system calls, e.g. Falco | what a process in a container actually did |
| Logs | containers and components | what the software reported |
| Metrics | kubelet, components, applications | how much, how often, how full |

The audit log knows nothing about what happens inside a container, and a runtime tool knows nothing
about API requests: you need both. Falco detects and alerts; it does not block. Audit logs contain
sensitive data and must be protected from general access.

### PKI

Kubernetes components authenticate each other with TLS certificates. kubeadm keeps them in
`/etc/kubernetes/pki`.

| CA | Signs |
|---|---|
| Kubernetes general CA (`ca.crt`) | API server serving certificate, client certificates of kubelets, controller manager, scheduler, kube-proxy, admins |
| etcd CA (`etcd/ca.crt`) | etcd server and peer certificates, the API server's etcd client certificate |
| Front-proxy CA (`front-proxy-ca.crt`) | the API server's client certificate towards extension API servers (aggregation layer) |

- Separate CAs keep a certificate issued for one purpose from being accepted for another.
- ServiceAccount tokens are signed with a key pair (`sa.key`, `sa.pub`), not with a CA.
- Whoever holds a CA's private key can issue any identity that CA is trusted for: guard the CA
  keys above everything else, and rotate certificates before they expire.

### Connectivity and service mesh

| Need | Mechanism | What it does not do |
|---|---|---|
| Limit who may connect | NetworkPolicy (network plugin enforces it) | encrypt, authenticate workloads |
| TLS for clients from outside | Ingress or Gateway with a TLS Secret | encrypt the hop to the Pods |
| Encrypt and authenticate service to service | mTLS in a service mesh | replace authorization in the application |

- **Ingress TLS** ends at the ingress controller: port 443, certificate from a Secret with `tls.crt`
  and `tls.key`, plain text from there to the Service and its Pods.
- A **service mesh** puts proxies into the data path. Its control plane issues each workload an
  X.509 certificate carrying its identity; the proxies perform mutual TLS, so both ends are
  authenticated and the traffic is encrypted, without changes to application code. On top of the
  identities the mesh can enforce which service may call which.
- Keep the API server, kubelet API and etcd off the internet, and filter access from Pods to the
  cloud metadata API.

## Common confusions

- **Signing and scanning**: a signature says who vouches for an image; a scan says what is known to
  be wrong inside it. A signed image can be vulnerable, a clean one can be unsigned.
- **Mutating and validating**: order matters, mutating first. Validation sees the final object.
- **Admission and authorization**: RBAC decides whether a subject may perform a verb on a resource;
  admission judges the content of the object. Only RBAC covers reads.
- **TLS and mTLS**: with TLS only the server proves its identity; with mutual TLS the client
  presents a certificate too.
- **NetworkPolicy and mesh policy**: IP and port level against identity level. They complement each
  other.
- **Audit log and `audit` mode**: Pod Security's `audit` mode writes an annotation into the audit log; it is
  not the audit log itself and it blocks nothing.
