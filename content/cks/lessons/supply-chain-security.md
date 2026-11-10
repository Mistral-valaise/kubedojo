# Supply Chain Security

**20% of the exam.** Know what is in your images, where they come from, and what gets admitted.

## The 20% that solves 80%

### Images: small, pinned, non-root

```dockerfile
FROM golang:1.24 AS build            # build stage: compilers and sources stay here
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /app

FROM gcr.io/distroless/static:nonroot   # runtime stage: no shell, no package manager
COPY --from=build /app /app
USER 65532
ENTRYPOINT ["/app"]
```

Checklist for "harden this Dockerfile" tasks:

- pin the base image — a version tag, never `latest`; a digest is strongest
- no secrets in `ENV`, `ARG` or files copied into a layer (layers are forever)
- a non-root `USER` as the **last** USER instruction
- remove package managers' caches and tools the app does not need; fewer layers
- `COPY` instead of `ADD`

Pin by digest in manifests so the image cannot change under you:

```yaml
image: nginx@sha256:4b0c...            # the tag is optional and ignored when a digest is given
```

`k get pod <p> -o jsonpath='{.status.containerStatuses[*].imageID}'` shows the digest that is running.

### Scan and inventory

```bash
trivy image --severity HIGH,CRITICAL nginx:1.27-alpine          # known vulnerabilities
trivy image --format spdx-json  -o sbom.spdx.json nginx:1.27-alpine   # SBOM (SPDX)
trivy image --format cyclonedx  -o sbom.cdx.json  nginx:1.27-alpine   # SBOM (CycloneDX)
trivy sbom sbom.cdx.json                                         # scan an SBOM later
```

An SBOM lists every package and version in an artifact. `jq` answers questions about it:

```bash
jq -r '.packages[] | select(.name=="zlib") | .versionInfo' sbom.spdx.json
```

### Static analysis of manifests

Read manifests like an attacker. Findings that always count: `privileged: true`, `hostPath`,
`hostNetwork`/`hostPID`, running as root, `latest` tags, secrets in `env`, missing resource limits,
`allowPrivilegeEscalation` not set to false. Tools such as `kubesec` and `trivy config` automate this.

### Admission: only trusted images

**ValidatingAdmissionPolicy** (built in, CEL expressions):

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata: {name: trusted-registries}
spec:
  failurePolicy: Fail
  matchConstraints:
    resourceRules:
      - {apiGroups: [""], apiVersions: [v1], operations: [CREATE, UPDATE], resources: [pods]}
  validations:
    - expression: "object.spec.containers.all(c, c.image.startsWith('registry.k8s.io/'))"
      message: "images must come from registry.k8s.io"
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata: {name: trusted-registries}
spec:
  policyName: trusted-registries
  validationActions: [Deny]
  matchResources:
    namespaceSelector:
      matchLabels: {kubernetes.io/metadata.name: shop}
```

A policy does nothing without a binding. Older clusters solve the same with the `ImagePolicyWebhook`
admission plugin or with policy engines (OPA Gatekeeper, Kyverno).

## Habits that save points

- `k run test --image=<img> --dry-run=server` checks an admission policy without creating a Pod.
- Remember `initContainers` and `ephemeralContainers` when a policy must cover *all* images.
