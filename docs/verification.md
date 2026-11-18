# Verifying content

Every question ships with a grader, and a grader that is wrong is worse than none: a false negative
tells a learner that a correct answer is wrong. `kubedojo content test` proves each question against
real clusters, and CI runs it. This page explains what is proven, how to run it and how to read a
failure.

## What is proven

For every question, on a real kind cluster:

1. `setup.sh` runs.
2. **Every check fails** on the untouched environment — except checks marked `guard: true`, which
   protect against a destructive shortcut and are allowed to pass.
3. `solution.sh` runs and exits 0.
4. **Every check passes** within `--settle` (default 150s).
5. The checks are run **twice more** without any change and must give the same grade.
6. With `--repeat N` the question is reset and steps 1–5 are done again, N times in total. This finds
   checks that depend on timing and setups that leave something behind.
7. Each script under `variants/` runs on a freshly reset question and must produce **exactly** the
   grade it declares (see [Variants](#variants)).

`--exam` proves the other path: all questions of a track are set up together in one exam session, all
solutions are applied and the exam is graded by the same code that grades a learner's exam. It must
reach a perfect score. This finds questions that disturb each other on a shared cluster.

## Run it

```bash
make images go                                          # once: sandbox images and the binary
./bin/kubedojo content lint                             # no cluster needed
./bin/kubedojo content test --question audit-log-policy # one question, about 1–10 minutes
./bin/kubedojo content test --track cks --report cks.json
./bin/kubedojo content test --track cks --repeat 2      # also prove resets
./bin/kubedojo content test --exam --track cks          # all questions together, as in an exam
```

| Flag | Meaning |
|---|---|
| `--track`, `--question a,b` | limit the run |
| `--repeat N` | prove each question N times, resetting in between (default 1) |
| `--no-variants` | skip `variants/` |
| `--settle 150s` | how long checks may take to pass after the solution ran |
| `--exam` | one exam session with every question of the track |
| `--report file.json` | write the machine-readable report |
| `--keep` | keep the sandbox to look around |
| `-v` | print provisioning progress |

The command exits non-zero when any question is not `ok`.

## The report

`--report` writes one JSON document. It is rewritten after every question, so an interrupted run still
leaves a usable report.

```json
{
  "environment": {"kubernetesVersion": "v1.37.0", "nodeImage": "kubedojo/node:v1.37.0", "driver": "kind", "os": "linux", "arch": "amd64", "repeat": 1},
  "summary": {"ok": 20, "false-negative": 1},
  "questions": [{
    "id": "audit-log-policy", "track": "cks", "domain": "runtime-security", "status": "ok", "seconds": 412.3,
    "runs": [{
      "run": 1, "script": "solution.sh", "expect": "pass", "status": "ok",
      "before": {"score": 0, "max": 4, "checks": [...]},
      "after":  {"score": 4, "max": 4, "checks": [...]},
      "settledSeconds": 12.1, "rechecks": 2
    }]
  }]
}
```

Each entry of `checks` carries what a failure needs to be understood: `command`, `on` (the machine),
`expected`, `observed`, `exitCode`, `stderr`, `durationMs`, `attempts` and `timedOut`. `stderr` is
clipped, and tokens, keys and passwords are replaced by `[redacted]`.

| `status` | Meaning | Usual cause |
|---|---|---|
| `ok` | proven | |
| `false-positive` | a check passes although the task is not solved, or a wrong variant passes | the check counts something that is 0 when the command fails; it does not test the negative |
| `false-negative` | a check fails although the reference solution (or a valid variant) ran | old state decides the grade; the check is too strict; `solution.sh` does not wait |
| `flaky` | a check passed and then failed with nothing changed | fixed sleeps, log windows, probes with short timeouts |
| `solution-error` | `solution.sh` or a variant exited non-zero | |
| `setup-error` | the sandbox or `setup.sh` failed | |
| `skipped` | not runnable in this run; `reason` says why | the question needs another driver |

Nothing is skipped silently: skipped questions are listed in the output and in the report.

## Diagnose a failure

```bash
./bin/kubedojo content test --question <id> --keep --report out.json
jq '.questions[0].runs[-1].after.checks[] | select(.passed | not)' out.json
docker exec -it -u candidate kd-<session>-terminal bash        # the learner's view
docker exec -it <alias>-control-plane bash                     # where checks run, as root
```

Copy the `command` of the failing check and run it on the machine named in `on`. Checks run with
`bash -c` as root (as `candidate` on the terminal) with `KUBECONFIG=/etc/kubernetes/admin.conf`.

## Variants

A question can ship more answers than the reference solution, as scripts in `variants/`. The header
declares the grade:

```bash
# expect: pass                 another valid solution — every check must pass
# expect: fail nothing-else    a plausible wrong answer — exactly these checks must fail
```

Use them where a wrong answer is tempting or where the grader once got it wrong. For example
`content/cks/questions/audit-log-policy/variants/` holds a policy with a catch-all at `Metadata` (must
fail `nothing-else`), Secrets logged with bodies (must fail `secrets`), and a policy that is corrected
after a first, too broad attempt (must pass — the log still contains the old lines).

## Lint warnings

`content lint` warns about check patterns that have produced wrong grades. `--strict` turns the
warnings into errors; CI uses it.

- **reads the tail of a log file** — old lines decide the grade. Note where the log ends, cause the
  events, wait for a marker, and look only at the lines in between.
- **uses a fixed sleep** — poll for the state in a loop, or set `retry`.
- **expects "0" from a pipeline that cannot fail** — `... | wc -l` prints 0 when the command in front
  failed. Assert the precondition first and `exit 1`.
- **starts a throwaway Pod with kubectl run --rm** — when the command in the Pod finishes before
  kubectl has attached, its output is lost and the check fails although the cluster is fine. Probe
  with `kubectl exec` in a Pod the setup created, or with `curl` from the node.

## CI and the supported matrix

| Job | When | What |
|---|---|---|
| `build` | every push and pull request | unit tests, `content lint --strict`, export, typecheck |
| `content` (cka, cks) | pushes to `main`; pull requests that touch content, the grader, the session code or the sandbox, or carry the `content` label | `content test --track <t> --report`, report uploaded as an artifact |
| `content-nightly` (cka, cks) | every night and on demand | `content test --repeat 2`, then `content test --exam` |
| `content-next` | on demand | the same proof against another Kubernetes version |

Supported and tested: Kubernetes v1.37 (`kindest/node:v1.37.0`), kind v0.33.0, Docker 27+ on
linux/amd64 (CI) and darwin/arm64 (development). One track needs about 8 GB of memory for Docker.

Runtime: a single question takes 1–3 minutes, questions with `freshEnvironment` up to 10 because the
cluster is recreated on every reset; a track takes 30–60 minutes, and `--repeat 2` about twice that.
The CI jobs have a 90 minute limit (150 for the nightly job).

## Known limits

- Tasks that need a real kernel boundary (AppArmor, gVisor, Falco) cannot run on kind and are not part
  of the question bank yet. See [cks-coverage.md](cks-coverage.md).
- A check runs for at most 60s unless it sets `timeout` (up to 5m).
- Exam grading re-runs failed checks for 20s in total, so an answer given in the last seconds of an
  exam is graded once it has converged. State that needs longer than that is graded as it is.
