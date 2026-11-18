# Curriculum design: the road to Kubestronaut

Why kubedojo has the tracks it has, in the order it recommends, and where each one stops. Written
for v0.4.0, which added Linux & Vim Essentials, KCNA and KCSA to the three hands-on tracks.

**Sources.** Everything below that describes an exam or a program comes from the official pages
listed at the end, read on 2026-11-18. Curricula change; check the pages before relying on a number.
kubedojo is an independent project: it is not affiliated with or endorsed by the CNCF or the Linux
Foundation, its questions are original, and no exam content is reproduced.

## The goal the path leads to

The CNCF's Kubestronaut program recognises people who hold five certifications at the same time:

| Exam | Kind | Format | Pass mark |
|---|---|---|---|
| KCNA | theory | 60 multiple-choice questions, 90 minutes, closed book | 75% |
| KCSA | theory | 60 multiple-choice questions, 90 minutes, closed book | 75% |
| CKAD | hands-on | 2 hours, 15 to 20 tasks on live clusters | 66% |
| CKA | hands-on | 2 hours, 15 to 20 tasks on live clusters | 66% |
| CKS | hands-on | 2 hours, 15 to 20 tasks on live clusters; candidates must have passed the CKA | 67% |

kubedojo has one track for each, and a sixth that prepares for the three hands-on ones.

## The order

`content/path.yaml` recommends: **Linux & Vim → KCNA → CKAD → CKA → KCSA → CKS**.

- **Linux & Vim first.** The hands-on exams are an ssh session: every task names a host to connect
  to, `sudo -i` for root, a terminal editor for every manifest, `man` pages, and `curl` or `wget`.
  The official exam instructions even tell candidates how to enter and leave insert mode. Someone
  who fights the shell and the editor loses the time the Kubernetes tasks need.
- **KCNA before the hands-on exams.** It is the vocabulary: what the components are and what the
  ecosystem around them is called. It is also the cheapest exam to pass first.
- **CKAD before CKA.** Using a cluster comes before operating one. CKAD stays inside namespaces;
  CKA adds the control plane, nodes, etcd and certificates.
- **KCSA next to CKS.** The security theory is most useful right before it is practised, and CKS
  requires a passed CKA anyway.

The order is a recommendation. Nothing is locked: every track is open from the first minute. The
home page shows the path with the learner's readiness per step and how many of the five mock exams
they have passed. Linux counts as a foundation, not toward the five.

## Linux & Vim Essentials

**What it is.** A skills track of 40 exercises in 10 modules, solved on a plain Linux host. It has no
exam behind it: its "skills assessment" (15 exercises, 60 minutes, 70%) is kubedojo's own measure,
and the numbers are a judgment, not a published standard.

**What it is not.** An LFCS simulator. The LFCS curriculum today is system administration: kernel
parameters, package management, LVM and filesystems, packet filtering and NAT, SELinux, virtual
machines, LDAP. It no longer even lists navigation, text processing, pipes or an editor as
competencies; it assumes them. Those assumed skills are exactly this track.

| Module | Weight | Exercises | In scope |
|---|---|---|---|
| Shell & navigation | 8% | 3 | `find`, `type`, PATH lookup, `man` and `--help` |
| Files & directories | 8% | 3 | globbing, safe removal, hard and symbolic links |
| Text, search & logs | 10% | 4 | `grep`, `cut`, `sort`, `uniq`, following a live log |
| Pipes & transformations | 12% | 5 | redirection, `xargs`, `sed`, `awk`, `diff`, `jq` |
| Vim | 20% | 8 | modes, search and replace, blocks, YAML indentation, recovery |
| Permissions & identity | 8% | 3 | modes, ownership, setgid and sticky directories, ACLs |
| Processes, signals & services | 10% | 4 | signals, job control, `service`, who listens where |
| Networking & SSH | 8% | 3 | DNS versus TCP versus HTTP, `ssh`, `scp`, bind addresses |
| Bash automation | 8% | 3 | quoting, exit codes, loops, scripts graded on unseen input |
| Incident troubleshooting | 8% | 4 | disk versus inodes, log correlation, a support bundle |

Vim has the largest share on purpose: it is the one tool every hands-on task goes through.

**Changes against the first proposal** (issue #9):

- *Processes & services* became *Processes, signals & services* without `systemctl` exercises. The
  sandbox is a container without systemd. A fake `systemctl` would teach a tool that lies, so the
  exercises use real processes, signals and Debian's `service` scripts, and the lesson shows the
  systemd equivalents. The CKA track's nodes run real systemd for those who want it.
- *Disk full versus out of inodes* is a diagnosis on a directory tree plus recorded `df` output.
  A container cannot mount a filesystem to exhaust for real.
- *Copying between machines* uses real ssh to a second account on the host. A solution script runs
  on one machine only, so a copy to the jump host could not be proven automatically.

**How it is graded.** By outcome only: the file that results, the state of permissions or
processes, the output of the learner's script on input they have not seen. Never by command, editor
or keystroke. Most exercises carry variant scripts that prove it: another correct solution with
different tools must pass, and plausible wrong ones (forgot hidden files, unquoted variable,
`chmod 777`) must fail exactly the checks named.

**Later, not now.** An advanced, LFCS-oriented extension would need a VM driver: storage and LVM,
packet filtering, SELinux, package management, unit files and journald, Git. `sandbox.Driver` is the
extension point. None of it is in this release.

## KCNA and KCSA

Theory tracks: multiple-choice questions, no sandbox, no Docker. Domains and weights are the
official ones.

| KCNA domain | Weight | Questions |
|---|---|---|
| Kubernetes Fundamentals | 44% | 18 |
| Container Orchestration | 28% | 12 |
| Cloud Native Application Delivery | 16% | 7 |
| Cloud Native Architecture | 12% | 6 |

| KCSA domain | Weight | Questions |
|---|---|---|
| Overview of Cloud Native Security | 14% | 6 |
| Kubernetes Cluster Component Security | 22% | 9 |
| Kubernetes Security Fundamentals | 22% | 9 |
| Kubernetes Threat Model | 16% | 6 |
| Platform Security | 16% | 6 |
| Compliance and Security Frameworks | 10% | 5 |

**Mock exam size.** 30 questions in 45 minutes at 75%: the real pace of 1.5 minutes per question,
at half the length, so that two mock exams drawn from a bank of about 40 differ. The track card
says so. A bank of about 90 would allow the full 60 in 90.

**Single answer.** The official pages do not say whether the real exams contain multi-select
questions. kubedojo's questions have exactly one correct option. The format supports several.

**How the answers were checked.** A program cannot prove that an answer key is right; `content test`
only proves that the key is what gets graded. So every question was written from a fetched official
page and cites it, and a second reader answered every question blind, without the key, before
comparing: 37 of 37 for KCNA and 38 of 38 for KCSA matched. That reading still changed thirteen
questions, where an explanation claimed more than its source said or a stem relied on a default.

**One honest oddity.** The KCSA curriculum lists "The 4Cs of Cloud Native Security", a model the
Kubernetes documentation dropped after v1.28. The question on it says so and cites the old source.

## What the path does not cover

- **Golden Kubestronaut** asks for every CNCF certification and LFCS. Out of scope.
- **Kernel-level security tasks of the CKS** (AppArmor, gVisor, Falco) cannot run on kind and are
  not in the CKS track; see [verification.md](verification.md).
- **Passing here is not passing there.** A mock exam uses kubedojo's questions and kubedojo's
  environment. It says whether the material sits, not what the proctored exam will ask.

## Sources

Read 2026-11-18.

- CNCF curricula: <https://github.com/cncf/curriculum> (CKA, CKAD and CKS v1.37; KCNA; KCSA)
- Kubestronaut program and FAQ: <https://www.cncf.io/training/kubestronaut/>
- Certification pages: <https://training.linuxfoundation.org/certification/> (KCNA, KCSA, CKA, CKAD,
  CKS, LFCS)
- Exam instructions and allowed resources:
  <https://docs.linuxfoundation.org/tc-docs/certification/tips-cka-and-ckad>,
  <https://docs.linuxfoundation.org/tc-docs/certification/important-instructions-cks>,
  <https://docs.linuxfoundation.org/tc-docs/certification/certification-resources-allowed>,
  <https://docs.linuxfoundation.org/tc-docs/certification/important-instructions-mc>
- Trademarks: <https://www.linuxfoundation.org/legal/trademarks>. Kubernetes, CKA, CKAD and CKS
  are registered marks of the Linux Foundation; Kubestronaut is a mark of the Linux Foundation.
