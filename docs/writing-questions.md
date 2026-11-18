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
root with `KUBECONFIG` set; `$KD_HOST` holds the cluster's ssh name. In a container environment the
machines are `host` (default) and `terminal`, and there is no `resource` check; see
[Container exercises](#container-exercises).

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

## Container exercises

Tracks that teach the shell (`content/linux/`) run on one Linux container instead of a cluster: the
track's environment has `type: container`. A question looks the same; what differs is listed here.
`linux/find-large-logs` is the model, including its `variants/`.

**Machines.** `host` is the Linux container the learner reaches with `ssh {{host}}`; it is the
default for checks and scripts. `terminal` is the jump host, for tasks about `ssh`, `scp` or `rsync`
between two machines. `{{worker}}` does not exist. Checks are `file` and `exec`; a `resource` check is
rejected by the loader.

**Users.**

| What | Runs on | As | Environment |
|---|---|---|---|
| `setup.sh`, `cleanup.sh` | `host` | root | `KD_HOST`, `KD_QUESTION` |
| checks | `host` (or `on: terminal`) | root (`candidate` on the terminal) | the same |
| `solution.sh`, `variants/*.sh` | `host` | **`candidate`**, starting in `/home/candidate` | the same |
| the learner | `ssh {{host}}` | `candidate`, passwordless `sudo` | a login shell: no `KD_*` |

The solution runs as `candidate` so that what it leaves behind is owned as a learner's work would be.
Consequences: `setup.sh` must give `candidate` access to what the task changes (`chown`), or the task
must need `sudo` on purpose and the solution must use it; and a check about ownership or modes
grades what a learner really produces. There is no `KUBECONFIG` and no `kubectl` on the host.

**One container for all exercises.** In an assessment every setup of the track runs in the same
container, about forty of them. So each exercise keeps to itself:

- it owns **`/opt/course/<question-id>/`** and nothing else in the file system that another
  exercise could want; `setup.sh` starts with `rm -rf` of that directory and recreates it,
- every **user, group, process name, service or file under `/etc`** it creates is prefixed with the
  question id (or a short form of it: user names end at 32 characters),
- every **port** it listens on is one from 20000 to 29999 that no other exercise uses,
- it does not change what is shared: `candidate`'s shell startup files, `/etc/hosts` entries of
  others, sshd, the system-wide Vim configuration.

Check that names and ports are free before you pick them:

```bash
ls content/linux/questions                                   # ids, and with them directories and prefixes
grep -rhoE '\b2[0-9]{4}\b' content/linux/questions | sort | uniq -c   # ports in use
```

`content test --exam` proves that the exercises do not disturb each other.

**No systemd.** PID 1 is an init that only reaps processes; `systemctl` does not work. For exercises
about processes and services use real processes, signals, jobs, `/proc`, `ss`, and Debian's
`service <name> start|stop|status` with the scripts in `/etc/init.d` (`nginx`, `cron` and `ssh` are
installed; only sshd runs).

**`kd-fixture`** starts and removes the background processes of an exercise:

```bash
kd-fixture reset                                              # first line of a setup.sh that starts processes
kd-fixture start web -- python3 -m http.server 20080 --bind 127.0.0.1 --directory /opt/course/my-id/www
kd-fixture start --user my-id-app worker -- /opt/course/my-id/worker.sh
kd-fixture pid web                                            # for a check: is that very process gone?
kd-fixture log web                                            # path of the file its output goes to
```

`start` detaches the command (own session, output to a log file), so `setup.sh` returns, and fails
if the command exits at once. `reset` ends the fixtures of the current question and every other
process that carries `KD_QUESTION=<id>` in its environment, whoever owns it: that is what makes a
setup idempotent, and it also removes what a solution left running. A plain `cmd &` in `setup.sh`
keeps `docker exec` from returning; use `kd-fixture start`.

**Things that differ from a real machine.**

- `/etc/hosts`, `/etc/resolv.conf` and `/etc/hostname` are bind mounts. They can be edited in place
  (`vim`, `echo ... >>`, `sed -i` **fails**: it replaces the file by renaming, which a mount point
  does not allow). Use `sed ... > tmp && cat tmp > /etc/hosts` in scripts, and do not ask the learner
  for `sed -i` on these files.
- DNS is Docker's resolver at `127.0.0.11`: `dig` and `getent hosts` resolve the terminal
  (`kd-<session>-terminal`) and internet names; there is no local zone to manage.
- The container has 1 GB of memory, 512 processes and 2 CPUs. An exercise that needs a "runaway"
  process should make it obvious, not heavy.
- Kernel-level tasks do not work: `sysctl -w`, `mount`, `modprobe`, `iptables`, changing the clock.
- Time moves: ages in `setup.sh` are relative (`touch -d '45 days ago'`), and checks do not compare
  against a date.

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
