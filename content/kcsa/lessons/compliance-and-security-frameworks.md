# Compliance and Security Frameworks

**10% of the exam.** Which framework is for what, how its parts are called, and which tools automate
the checking. You are asked to match a need to a framework or a tool, not to quote control numbers.

## The 20% that solves 80%

### What kind of document is it?

| Kind | Says | Examples |
|---|---|---|
| Regulation or standard with audits | what an organization must achieve | PCI DSS, HIPAA, GDPR, ISO/IEC 27001, SOC 2 |
| Control catalog | which controls exist to choose from | NIST SP 800-53 |
| Technology guidance | which risks a technology has and what to do about them | NIST SP 800-190 |
| Benchmark | which concrete settings a product should have | CIS Benchmarks |
| Threat framework | how attackers behave, how to find threats | MITRE ATT&CK, STRIDE |
| Supply chain framework | how trustworthy the production of an artifact is | SLSA, in-toto |

- **NIST SP 800-53**: a catalog of security and privacy controls for information systems and
  organizations in general.
- **NIST SP 800-190**: the Application Container Security Guide, explaining the security concerns of
  containers and recommending how to address them.
- **CIS Benchmarks**: prescriptive configuration recommendations, developed by consensus among
  security experts. The CIS Kubernetes Benchmark addresses control plane components and nodes:
  flags, file permissions, policies.

Compliance is evidence that required controls are in place. It is not the same as being secure, and
a framework never configures anything by itself.

### Threat modelling

Threat modelling asks, for a system drawn as a data flow diagram with its trust boundaries: what can
go wrong here, and what do we do about it?

**STRIDE** classifies threats; each category violates one security property:

| Category | Violates | Kubernetes example |
|---|---|---|
| **S**poofing | authentication | using a stolen ServiceAccount token |
| **T**ampering | integrity | changing an image in the registry, editing data in etcd |
| **R**epudiation | non-repudiation | denying an action when no audit log exists |
| **I**nformation disclosure | confidentiality | reading Secrets, sniffing unencrypted traffic |
| **D**enial of service | availability | exhausting node memory or flooding the API |
| **E**levation of privilege | authorization | escaping from a container to the node |

**MITRE ATT&CK** is a knowledge base of observed adversary behaviour, with a matrix for containers:

| Term | Meaning |
|---|---|
| Tactic | the "why": the adversary's goal (the columns, e.g. Persistence) |
| Technique | the "how": a way to reach that goal (the cells, e.g. Escape to Host) |
| Sub-technique | a more specific description of a technique |
| Procedure | the specific implementation an adversary used |

STRIDE is a way of thinking while you design; ATT&CK is a catalog to check detection and defences
against what attackers really do.

### Supply chain compliance

| Thing | What it is |
|---|---|
| **SBOM** | list of the components in an artifact. Formats: SPDX (Linux Foundation, ISO/IEC 5962) and CycloneDX (OWASP, ECMA-424) |
| **Provenance** | verifiable information on where, when and how an artifact was produced |
| **Signature** | proof that an identity signed this exact artifact (Sigstore: Cosign, Fulcio, Rekor) |
| **VEX** | statement whether known vulnerabilities actually affect a product |
| **in-toto** | framework that makes transparent which steps were performed, by whom and in what order |
| **SLSA** | levels of increasing supply chain security guarantees |

SLSA build levels in one line each:

- **L0**: no guarantees.
- **L1**: provenance exists and shows how the package was built.
- **L2**: a hosted build platform generates and signs the provenance.
- **L3**: the build platform is hardened; tampering during the build is prevented.

An SBOM answers "what is inside?", provenance answers "how was it made?", a signature answers "who
vouches for it?". When a new CVE appears, the SBOM tells you which artifacts contain the component.

### Automation and tooling

| Tool | Checks | When |
|---|---|---|
| kube-bench | cluster configuration against the CIS Kubernetes Benchmark | on the nodes, on demand or scheduled |
| Trivy and other scanners | images for known vulnerabilities, manifests for misconfiguration | CI, registry, cluster |
| Kyverno, OPA Gatekeeper | API objects against policies written as code | at admission, plus reports on existing objects |
| Pod Security admission | Pods against the Pod Security Standards | at admission |
| Falco | behaviour of running processes against rules | at runtime |
| Cosign | signatures and attestations of artifacts | CI and admission |
| Audit logging | records who did what through the API | continuously, as evidence |

Policy as code turns a written requirement into something that is enforced on every request and
whose results can be handed to an auditor. Automated checks give continuous compliance instead of a
snapshot from the day of the audit.

## Common confusions

- **Benchmark and framework**: CIS tells you which setting to use; NIST SP 800-53 or ISO 27001
  tell you which controls an organization needs. A benchmark is often the evidence for a control.
- **Tactic and technique**: goal against method. "Privilege Escalation" is a tactic.
- **Repudiation and spoofing**: denying your own action against acting as someone else.
- **SBOM and provenance**: content against origin. Neither is a signature.
- **SLSA and SBOM formats**: SLSA rates the build process; SPDX and CycloneDX are file formats.
- **kube-bench and Kyverno**: one inspects how the cluster is configured, the other judges the
  objects that are sent to the API.
- **Detection and prevention**: Falco and audit logs tell you; admission policies stop it.
