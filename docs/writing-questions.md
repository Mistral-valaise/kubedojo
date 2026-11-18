# Writing questions

There are two kinds of question. A **task** is solved in a terminal and graded by checks; most of this
page is about tasks. A **multiple-choice question** belongs to a theory track (`kind: theory`) and is
one YAML file: see [Multiple-choice questions](#multiple-choice-questions).

A task is a directory: `content/<track>/questions/<id>/`.

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

## Multiple-choice questions

Tracks with `kind: theory` in `track.yaml` (KCNA, KCSA) prepare for multiple-choice exams. They have no
environments, and each question is one file: `content/<track>/questions/<id>.yaml`. The file name is the
id. Nothing is set up and nothing runs: a learner picks options and the answer is compared with the key.

```yaml
title: Placing new Pods on nodes        # shown in lists: must not give the answer away
domain: kubernetes-fundamentals         # a domain id from track.yaml
difficulty: easy                        # easy | medium (default) | hard
tags: [control-plane, scheduling]
stem: |-                                # the question, markdown
  A Deployment is scaled from 2 to 5 replicas. The three new Pods exist in the API server, but their
  `spec.nodeName` is still empty.

  Which component decides on which node each of them will run?
options:                                # 3 to 6, the exams use 4; shown in random order
  - text: kube-scheduler
    correct: true
  - text: kube-controller-manager
    explain: >-                         # why this option is wrong; shown in the review
      Its ReplicaSet controller created the three Pod objects. Creating a Pod and choosing a node for
      it are separate jobs.
  - text: kubelet
    explain: A kubelet starts the containers of Pods that are already bound to its own node.
  - text: kube-proxy
    explain: kube-proxy programs the forwarding rules for Services. It takes no part in placing Pods.
explanation: >-                         # required: why the answer is right
  The kube-scheduler watches the API server for Pods that have no node assigned, filters the nodes that
  can run each Pod, scores the rest and tells the API server its choice.
references:                             # required: the page that backs the answer
  - title: Kubernetes Scheduler
    url: https://kubernetes.io/docs/concepts/scheduling-eviction/kube-scheduler/
```

Exactly one option is `correct`. With `multiple: true` the question is "select all that apply": at
least two options are correct, not all of them, and the answer counts only when it is exactly the
correct set. kubedojo's KCNA and KCSA questions are single-answer; the official exam pages do not say
whether theirs always are.

A question is worth one point, right or wrong. Every option gets an id derived from its text, so
reordering options changes nothing, and rewording one makes it a new option for sessions that are open.

### What lint checks

`kubedojo content lint` rejects a question that

- has an unknown field, no `title`, no `stem`, an unknown `domain` or a `difficulty` other than easy,
  medium or hard;
- has fewer than 3 or more than 6 options, an option without text, or the same option twice
  (compared without case and extra spaces);
- does not have exactly one correct option (or, with `multiple: true`, has fewer than two correct
  options or only correct ones);
- has no `explanation`, or no reference with a title and an `https://` URL.

It warns (and `--strict`, which CI uses, fails) when

- the question does not have 4 options, which is what the exams use;
- an option refers to the others or to its position ("all of the above", "none of these", "both",
  "option B", "above", "below"): options are shuffled for every session;
- the title contains the text of the correct option: titles are shown in question lists and in the
  exam navigator;
- the correct option is much longer than every wrong one: the length gives it away.

### Writing good distractors

A question tests knowledge only if the wrong options are tempting for someone who does not have it.

- **Ask one thing.** The stem describes a situation and ends in one question. It must be answerable
  before looking at the options, and it never contains a negation that is easy to miss ("which is
  NOT ..."): ask for the one thing that is true.
- **Every wrong option is a real misconception.** Use a neighbouring component, the default that
  people assume, the behaviour of the older API, the setting that does something similar. Not a joke,
  and not something from another field. If you cannot write an `explain` for why somebody would pick
  it and why it is wrong, replace it.
- **Exactly one option is defensible.** Read each wrong option as its advocate. If it is right under
  an assumption the stem does not rule out, tighten the stem: say what must hold instead of asking
  what "helps".
- **Same shape for all options.** The same grammatical form, about the same length and the same level
  of detail. The correct option must not be the only long one, the only one with a qualifier, or the
  only one that repeats a word from the stem.
- **No absolutes as a tell.** "always", "never", "only" mark wrong options in badly written tests;
  use them only where the documentation does.
- **Write the `explain` of every wrong option.** One or two sentences: what the option actually is or
  does. The review shows it under the option a learner picked, which is where most of the learning
  happens. An `explain` on the correct option is allowed and rarely needed; `explanation` covers it.
- **The explanation teaches, it does not repeat.** Say why the answer is right, in a way that would
  answer the next question on the topic too.

### References

Every question names at least one page that backs the answer. Prefer, in this order: kubernetes.io
documentation, the documentation of the CNCF project in question, the specification or standard itself
(NIST, CIS, SLSA, OCI). No blogs, no training providers, no exam dumps. Link the most specific page and
check that it says what the explanation says: open it, do not rely on memory. The exam curricula
linked in `track.yaml` under `docs` say what the domains cover.

Never copy questions from an exam or from another question bank. Write them from the documentation.

### Test it

```bash
kubedojo content lint --strict
kubedojo content test --track kcna            # no Docker needed
kubedojo content test --track kcna --exam
```

This proves that the grading works, not that the content is right; see
[verification.md](verification.md#multiple-choice-tracks). A second person reading every question
against its references is part of the work.

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
node details are skipped and listed, and so are multiple-choice questions. The export is generated, not run on Killercoda — try each scenario
there before publishing.
