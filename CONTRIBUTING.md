# Contributing to kubedojo

Thank you for helping people pass their exams. The most valuable contributions are **new questions**,
**fixes to existing ones** and **lesson improvements**.

## Ground rules

- **Original content only.** Do not submit questions copied or paraphrased from commercial exam
  simulators, courses or the real exams. Base them on the public CNCF curriculum and your own experience.
- A question is only accepted when `kubedojo content test` proves it: the checks fail on the starting
  state and pass after the reference solution.
- English is the project language for code, content and discussions.

## Set up

You need Go 1.26+, Node 22+, Docker and [kind](https://kind.sigs.k8s.io/).

```bash
make build            # UI + binary
make images           # sandbox images (first time only)
./bin/kubedojo up     # run it
```

For UI work, run the API and the dev server side by side:

```bash
make dev                      # API on :4400
cd web && npm run dev         # UI on :5173 with hot reload
```

## Adding or changing a question

Read [docs/writing-questions.md](docs/writing-questions.md), then:

```bash
./bin/kubedojo content lint
./bin/kubedojo content test --question <id>          # add --keep to inspect the sandbox
./bin/kubedojo content test --exam --track <track>   # when the question shares a cluster with others
```

Add the `content` label to your pull request so CI runs the full content proof.

## Changing code

```bash
make test             # Go unit tests + TypeScript type check
go vet ./... && gofmt -l cmd internal
helm lint deploy/helm/kubedojo
```

Keep pull requests focused, describe what you verified, and mention anything you could not test.

## Reporting problems

- A question that is wrong, unclear or graded unfairly: open a *Question problem* issue.
- A bug in the platform: open a *Bug report*.
- A security problem: see [SECURITY.md](SECURITY.md) — please do not open a public issue.
