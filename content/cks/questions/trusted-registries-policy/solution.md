```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata:
  name: trusted-registries
spec:
  failurePolicy: Fail
  matchConstraints:
    resourceRules:
      - apiGroups: [""]
        apiVersions: ["v1"]
        operations: ["CREATE", "UPDATE"]
        resources: ["pods"]
  validations:
    - expression: >-
        object.spec.containers.all(c,
          c.image.startsWith('registry.k8s.io/') ||
          c.image.startsWith('docker.io/library/'))
      message: images must come from registry.k8s.io/ or docker.io/library/
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata:
  name: trusted-registries
spec:
  policyName: trusted-registries
  validationActions: [Deny]
  matchResources:
    namespaceSelector:
      matchLabels:
        kubernetes.io/metadata.name: sec-robin
```

```bash
ssh {{host}}
k apply -f policy.yaml
k -n sec-robin run t --image=quay.io/prometheus/busybox --dry-run=server
# The pods "t" is invalid: : ValidatingAdmissionPolicy 'trusted-registries' with binding
# 'trusted-registries' denied request: images must come from registry.k8s.io/ or docker.io/library/
k -n sec-robin run t --image=registry.k8s.io/pause:3.10 --dry-run=server      # pod/t created (server dry run)
k -n default   run t --image=quay.io/prometheus/busybox --dry-run=server      # still allowed
```

New policies take a few seconds to become active.

**Division of labour:** the *policy* says **what** is valid, the *binding* says **where** and **how
strictly** (`Deny`, `Warn`, `Audit`). In production you would add `initContainers` and
`ephemeralContainers` to the expression and select Namespaces by a team label instead of a name.
