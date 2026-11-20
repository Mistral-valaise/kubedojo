# CKS curriculum coverage

How the CKS question bank maps to the official exam curriculum, what is missing and why.

- **Source:** *Certified Kubernetes Security Specialist (CKS) Exam Curriculum*, v1.37, published by
  the CNCF in [cncf/curriculum](https://github.com/cncf/curriculum) (`CKS_Curriculum v1.37.pdf`, last
  changed 2026-11-03). Exam facts (2 hours, performance based):
  [Linux Foundation CKS page](https://training.linuxfoundation.org/certification/certified-kubernetes-security-specialist/).
- **Verified:** 2026-11-17. The domains and weights in `content/cks/track.yaml` match the curriculum.
- All questions are original. They are written from the curriculum's objectives and the Kubernetes
  documentation, not from exam questions or commercial simulators.

## Domains

| Domain | Weight | Questions | In a 16-question exam |
|---|---|---|---|
| Cluster Setup | 15% | 5 | 2–3 |
| Cluster Hardening | 15% | 5 | 2–3 |
| System Hardening | 10% | 4 | 1–2 |
| Minimize Microservice Vulnerabilities | 20% | 5 | 3 |
| Supply Chain Security | 20% | 5 | 3 |
| Monitoring, Logging and Runtime Security | 20% | 5 | 3 |
| **Total** | 100% | **29** | 16 |

An exam draws questions per domain by weight, so every domain needs more questions than one exam
takes from it — otherwise every exam is the same. Before this review, System Hardening had 2 questions
and Runtime Security 3: exactly what one exam uses.

## Objectives

`+` marks questions added by this review.

### Cluster Setup (15%)

| Objective | Questions |
|---|---|
| Use Network security policies to restrict cluster level access | `default-deny-and-allow` |
| Use CIS benchmark to review the security configuration of Kubernetes components | `kubelet-anonymous-access`, `apiserver-admission-hardening` (cluster hardening) — findings are given as CIS check titles |
| Properly set up Ingress objects with TLS | `ingress-tls-termination` |
| Protect node metadata and endpoints | `+ metadata-egress-block`, `kubelet-anonymous-access` |
| Verify platform binaries before deploying | `verify-platform-binaries` |

### Cluster Hardening (15%)

| Objective | Questions |
|---|---|
| Use Role Based Access Controls to minimize exposure | `rbac-reduce-privileges`, `user-certificate-csr` |
| Exercise caution in using service accounts | `serviceaccount-no-token` |
| Restrict access to Kubernetes API | `+ anonymous-api-access`, `apiserver-admission-hardening` |
| Upgrade Kubernetes to avoid vulnerabilities | **gap** — see below |

### System Hardening (10%)

| Objective | Questions |
|---|---|
| Minimize host OS footprint | `rogue-listening-service` |
| Using least-privilege identity and access management | `+ host-account-lockdown` |
| Minimize external access to the network | `rogue-listening-service` |
| Appropriately use kernel hardening tools such as AppArmor, seccomp | `seccomp-localhost-profile`, `+ seccomp-default-kubelet`; AppArmor is a **gap** |

### Minimize Microservice Vulnerabilities (20%)

| Objective | Questions |
|---|---|
| Use appropriate pod security standards | `pod-security-restricted`, `readonly-root-filesystem`, `+ drop-capabilities` |
| Manage Kubernetes secrets | `secret-env-to-volume`, `etcd-encryption-at-rest` |
| Understand and implement isolation techniques (multi-tenancy, sandboxed containers) | **gap** — RuntimeClass with gVisor/Kata |
| Implement Pod-to-Pod encryption (Cilium, Istio) | **gap** |

### Supply Chain Security (20%)

| Objective | Questions |
|---|---|
| Minimize base image footprint | `dockerfile-hardening` |
| Understand your supply chain (SBOM, CI/CD, artifact repositories) | `sbom-generate`, `pin-image-digest` |
| Secure your supply chain (permitted registries, sign and validate artifacts) | `trusted-registries-policy`, `pin-image-digest`; signature verification is a **gap** |
| Perform static analysis of user workloads and container images | `+ manifest-static-analysis`, `dockerfile-hardening` |

### Monitoring, Logging and Runtime Security (20%)

| Objective | Questions |
|---|---|
| Perform behavioral analytics to detect malicious activities | `suspicious-process`, `+ container-drift`; Falco is a **gap** |
| Detect threats within physical infrastructure, apps, networks, data, users and workloads | `audit-privileged-pods`, `suspicious-process` |
| Investigate and identify phases of attack and bad actors within the environment | `+ audit-log-forensics` |
| Ensure immutability of containers at runtime | `+ container-drift`, `readonly-root-filesystem` |
| Use Kubernetes audit logs to monitor access | `audit-log-policy`, `+ audit-log-forensics` |

## Gaps and what blocks them

Sandboxes are kind clusters: the nodes are containers that share the kernel of the Docker host. That is
what makes them start in under a minute, and it rules out tasks that need their own kernel or a
specific CNI. These gaps are tracked here on purpose; the lessons cover the topics in text.

| Objective | Why there is no question | What would unblock it |
|---|---|---|
| AppArmor profiles | profiles are loaded into the kernel, which all sandboxes share with the Docker host; whether AppArmor is available at all depends on that host | the VM driver (`driver: vm`, see `internal/sandbox/sandbox.go`) |
| Falco rules and alerts | needs the eBPF probe or kernel module on a real kernel, and the Falco image in the node image | VM driver, Falco preinstalled |
| Sandboxed containers (gVisor, Kata) | `runsc` is not in the node image and does not run nested on all hosts | VM driver or `runsc` in the node image; a RuntimeClass question is then straightforward |
| Pod-to-Pod encryption (Cilium, Istio) | kind's default CNI is kindnet; installing Cilium or a mesh per sandbox costs minutes and gigabytes | an environment with Cilium preinstalled |
| Upgrade Kubernetes | `kubeadm upgrade` needs packages of two versions in the node image and takes 10+ minutes | a node image with the next patch release staged |
| Sign and validate artifacts (cosign) | needs a registry with signed images reachable from the sandbox; external services must not decide a grade | a local registry with fixtures in the sandbox, cosign in the terminal image |
| ImagePolicyWebhook | needs a webhook backend in the sandbox; `trusted-registries-policy` covers the objective with a ValidatingAdmissionPolicy | a small webhook fixture image |

## Rules for new CKS questions

- Runs on the `kind` driver with the tools in `images/node` and `images/terminal`; no downloads at
  grading time.
- Own Namespace (`sec-<bird>`) and own paths (`/opt/course/<topic>/`); questions in an environment
  share a cluster in exams.
- Checks test behaviour where possible (a request is denied, a process is confined) and pair every
  positive with a negative.
- At least one `variants/` script for the mistake the question is about.
- Proven with `kubedojo content test --question <id> --repeat 2` and in `--exam` — see
  [verification.md](verification.md).
