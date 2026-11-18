# Adding a track

A track is one exam: its domains, its lessons and its questions. kubedojo is driven by the content
directory, so the server and the web UI need no change for a new track. Seven places outside the
track's own directory do; they are listed at the end.

The CKAD track was added this way and is a good model: `content/ckad/`.

## 1. The directory

```text
content/<track>/
  track.yaml
  lessons/<domain-id>.md        one per domain
  questions/<question-id>/      see writing-questions.md
```

## 2. `track.yaml`

```yaml
id: ckad                                   # directory name, URL segment, prefix of cluster names
title: Certified Kubernetes Application Developer
short: CKAD                                # shown on cards and badges
description: >-
  One or two sentences for the track card.
kubernetesVersion: v1.37
exam:
  durationMinutes: 120
  passPercent: 66
  questions: 16                            # tasks in a full exam
domains:
  - id: application-design-build           # also the lesson's file name
    title: Application Design and Build
    weight: 20                             # percent of the exam; all weights sum to 100
    summary: One line for the track page.
environments:
  base:                                    # every question names one; "base" is the default
    workers: 1                             # one control plane plus this many workers
    description: General purpose cluster for application tasks.
```

Rules the loader enforces (`internal/catalog/catalog.go`):

- `exam.durationMinutes`, `exam.passPercent` and `exam.questions` are required.
- Domain weights sum to 100, and every domain has a lesson file.
- Every question's `domain` and `environment` exist in the track.

Take domains and weights from the exam's public curriculum, not from another training product.

**Environments** are kind clusters. A session creates each environment its questions need, so every
extra environment costs a cluster in an exam. Use a second one only for questions that would break
the others: control plane or node surgery (`ops` in CKA), cluster-wide security settings (`secure`
in CKS). `kindConfig` takes a raw kind configuration if generated nodes are not enough.

## 3. How many questions

A full exam draws `exam.questions` questions at random, split by domain weight
(`session.pickQuestions`). The tests require:

- at least `exam.questions` questions in the track,
- at least one question per domain,
- enough per domain to fill its share: `ceil(exam.questions × weight / 100)`.

More than the minimum makes two exams differ. CKAD has 22 questions for exams of 16.

All questions of an environment run together in an exam. Give each its own Namespace and its own
directory under `/opt/course/`, and check that the names are unused:
`grep -rhoE "app-[a-z]+|team-[a-z]+" content | sort | uniq -c`.

## 4. Tools the questions need

Questions run on the node image (`images/node`) and the terminal image (`images/terminal`). If the
track needs a tool neither has (CKAD needed `podman` to build images), add it to the Dockerfile and
**raise `LABEL dev.kubedojo.image-rev`** in it. The image tag only names the Kubernetes version;
`kubedojo doctor` compares the label and rebuilds a learner's cached image when it is older.

Not available on kind, so not usable in questions: AppArmor, gVisor, Falco, metrics-server, an
ingress controller, a registry to push to.

## 5. Wire it up

| File | Change |
|---|---|
| `content/embed.go` | add `all:<track>` to the `go:embed` line, or the track is not in the binary |
| `internal/catalog/catalog_test.go` | add the id to the list in `TestShippedContentIsValid` |
| `.github/workflows/ci.yaml` | add the id to the `track:` matrix of `content-nightly` (pull requests find tracks by themselves) |
| `.github/workflows/content-next.yaml` | add the id to the `track:` matrix |
| `cmd/kubedojo/main.go`, `deploy/helm/kubedojo/Chart.yaml` | the exam names in usage and chart description |
| `internal/tutor/tutor.go`, `internal/export/killercoda.go` | the exam names in the tutor prompt and the export intro |
| `README.md`, `docs/verification.md` | question counts and the CI table |

## 6. Prove it

```bash
make go
./bin/kubedojo content lint --strict
go test ./internal/catalog ./internal/session ./internal/export
./bin/kubedojo content test --track <track> --repeat 2    # every question, twice
./bin/kubedojo content test --track <track> --exam        # all together, as in an exam
```

While writing, `--content ./content` loads the directory instead of the embedded copy, so no rebuild
is needed between edits. See [verification.md](verification.md) for what the proof covers.
