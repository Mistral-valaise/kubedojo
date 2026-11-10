# Writing questions

A question is a directory: `content/<track>/questions/<id>/`.

| File | Purpose |
|---|---|
| `question.yaml` | metadata, the task text, hints and checks |
| `setup.sh` | creates the starting state, including anything that is broken on purpose |
| `solution.sh` | a reference solution a machine can run |
| `solution.md` | the worked solution a learner reads |
| `cleanup.sh` | optional; restores a healthy cluster (required for `disruptive` questions) |

## question.yaml

```yaml
title: RBAC for a pipeline ServiceAccount
domain: cluster-architecture        # a domain id from track.yaml
difficulty: easy                    # easy | medium | hard
minutes: 6                          # realistic time for a prepared candidate
environment: base                   # a cluster defined in track.yaml
tags: [rbac, serviceaccount]
task: |
  Solve this question on: `ssh {{host}}`

  Create a ServiceAccount named `pipeline` in Namespace `team-orca` ...
hints:
  - "First nudge: where to look."
  - "Second: the mechanism."
  - "Third: nearly the command."
checks:
  - id: sa
    title: ServiceAccount pipeline exists in team-orca
    type: resource
    resource: serviceaccount/pipeline
    namespace: team-orca
    explain: >-
      Why this matters and the usual mistake behind a failure.
```

Placeholders in `task`, `hints` and `solution.md`: `{{host}}` (the cluster's ssh name, e.g. `cka3827`),
`{{worker}}`, `{{worker2}}`. They differ per session.

Optional flags:

- `disruptive: true` — the question breaks a cluster-wide component (kubelet, scheduler, DNS). It needs
  a `cleanup.sh`; practice mode runs it before another question is prepared, and exams run disruptive
  setups last.
- `freshEnvironment: true` — the effects cannot be undone by a script (for example encrypting etcd).
  Resetting the question recreates the cluster.
- `driver: vm` — needs a real VM; hidden until a VM driver exists.

## Checks

Every check runs a command inside the sandbox and compares its output. `points` defaults to 1.

| `type` | Runs | Fields |
|---|---|---|
| `resource` | `kubectl get <resource> [-n ns] -o jsonpath=...` | `resource`, `namespace`, `jsonpath`, `absent: true` |
| `file` | `cat <path>` | `path` |
| `exec` | any shell command | `run` |

`on:` selects the machine: `control-plane` (default), `worker`, `worker2` or `terminal`. Commands run as
root with `KUBECONFIG` set; `$KD_HOST` holds the cluster's ssh name.

`expect:` matchers (all must hold): `equals`, `notEquals`, `contains: [..]`, `notContains: [..]`,
`matches: <regex>`, `oneOf: [..]`, `sameLines: [..]`, `sameItems: [..]`, `gte`, `lte`, `notEmpty`,
`exitCode`. Without `expect`, a check passes when the command exits 0.

Guidelines:

- **Check outcomes, not methods.** `kubectl auth can-i` beats inspecting a Role's YAML; a request that
  succeeds beats a field comparison. Accept every correct solution.
- **Check the negative too.** "can list Pods" needs "cannot delete Pods" next to it, or `cluster-admin`
  passes.
- **No free points.** No check may pass on the untouched starting state unless it guards something the
  learner could break — and then pair it with a condition that needs work.
- **`explain` teaches.** Say why the thing matters and what typically goes wrong, in two sentences.
- Use `run: |-` and `explain: >-` block scalars; YAML mangles `: ` and `#` in plain strings.

## Scripts

`setup.sh`, `cleanup.sh` and `solution.sh` run on the control plane node as root, with `KUBECONFIG` and
`$KD_HOST` set. Reach other nodes with `ssh ${KD_HOST}-worker '...'`.

- `setup.sh` must be **idempotent** — it is run again on every reset. Start by deleting what the question
  owns (`kubectl delete namespace ... --ignore-not-found --wait`).
- Give each question its **own Namespaces and paths** (`/opt/course/<topic>/`). In an exam, all
  questions of an environment share one cluster.
- Questions in an environment that contains disruptive questions must not depend on the scheduler,
  controller manager or DNS being healthy.
- Wait for what you break to take effect, so the learner does not see a healthy cluster for the first
  minute.
- Pin image tags, and prefer images other questions already use (`nginx:1.27-alpine`, `busybox:1.36`).

## Test it

```bash
kubedojo content lint
kubedojo content test --question my-new-question        # add --keep to inspect the sandbox afterwards
kubedojo content test --exam --track cka                # all questions together, as in an exam
```

`content test` runs `setup.sh`, requires the checks to **fail**, runs `solution.sh`, and requires every
check to **pass**. `--exam` sets up all questions of a track in one session, applies every solution and
expects a perfect score, which catches questions that disturb each other.

## Lessons

`content/<track>/lessons/<domain>.md` — one per domain. Keep the format: what the domain weighs, "the 20%
that solves 80%", and habits that save points. Commands over prose; a learner should finish it in five
minutes.
