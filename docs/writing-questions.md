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

Optional per check:

- `timeout: 120s` — how long one run may take (default 60s, at most 5m). Needed when the check starts a
  probe Pod or waits for a rollout.
- `retry: 20s` — run the check again until it passes or this time is over. For state that arrives on
  its own shortly after the action: a log line, an endpoint.
- `guard: true` — the check protects against a destructive shortcut ("the API server is healthy", "the
  base was not edited") and therefore passes on the untouched environment. `content test` requires
  every other check to fail before the task is solved.

Guidelines:

- **Check outcomes, not methods.** `kubectl auth can-i` beats inspecting a Role's YAML; a request that
  succeeds beats a field comparison. Accept every correct solution.
- **Check the negative too.** "can list Pods" needs "cannot delete Pods" next to it, or `cluster-admin`
  passes.
- **No free points.** No check may pass on the untouched starting state unless it guards something the
  learner could break — and then pair it with a condition that needs work.
- **A command that fails must not pass.** The exit code of a pipeline is that of its last command, so
  `kubectl ... | wc -l` prints `0` and exits 0 when `kubectl` failed. When the expected answer is a
  negative ("0", "blocked", "denied"), assert the precondition first and `exit 1`: the Namespace exists,
  the probe Pod can run a command, the file is there. Write the reason to stderr; it is shown to the
  learner.
- **Never grade history.** Do not `tail` a log and do not `sleep`. Note where the log ends, cause the
  events, wait in a loop for a marker to appear, then look only at the new lines
  (`cks/audit-log-policy` shows the pattern).
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
- A check never starts a throwaway Pod with `kubectl run --rm`: its output can be lost and the check
  fails at random. Let `setup.sh` create a long-lived client Pod and probe with `kubectl exec`, or use
  `curl` from the node against a ClusterIP. `content lint` warns about it.

## Test it

```bash
kubedojo content lint
kubedojo content test --question my-new-question --repeat 2   # add --keep to inspect the sandbox afterwards
kubedojo content test --exam --track cka                # all questions together, as in an exam
```

`content test` runs `setup.sh`, requires every check that is not a guard to **fail**, runs `solution.sh`,
requires every check to **pass** and to pass again when graded twice more. `--repeat 2` resets the
question and does it all again. `--exam` sets up all questions of a track in one session, applies every
solution and expects a perfect score, which catches questions that disturb each other.

Add scripts under `variants/` for other valid solutions (`# expect: pass`) and for tempting wrong ones
(`# expect: fail <check ids>`); each must produce exactly the declared grade. Details, the JSON report
and how to diagnose a failure: [verification.md](verification.md).

## Lessons

`content/<track>/lessons/<domain>.md` — one per domain. Keep the format: what the domain weighs, "the 20%
that solves 80%", and habits that save points. Commands over prose; a learner should finish it in five
minutes.

## Export to Killercoda

```bash
kubedojo content export --format killercoda --out killercoda
```

writes one [Killercoda](https://killercoda.com/creators) scenario per question: `setup.sh` becomes the
intro's background script, the checks become `verify.sh`, hints become collapsible sections and
`solution.md` the finish page. Questions that need two workers, a fresh cluster or kubedojo-specific
node details are skipped and listed. The export is generated, not run on Killercoda — try each scenario
there before publishing.
