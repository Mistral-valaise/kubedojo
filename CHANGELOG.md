# Changelog

Notable changes per release. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.4.0] - 2026-11-19

The road to Kubestronaut: three new tracks, a recommended path through all six, and two kinds of
practice that need no Kubernetes cluster.

### Added

- **Linux & Vim Essentials track.** 40 exercises in 10 modules with a lesson each: navigation, files,
  text and logs, pipes, Vim (8 exercises), permissions, processes and services, networking and ssh,
  bash, incident troubleshooting. A skills track, not a certification: its assessment is 15
  exercises in 60 minutes.
- **A Linux sandbox without a cluster.** Environments of `type: container` run on one unprivileged
  Linux container with memory, CPU and process limits, on a network of its own. It is ready in about
  a second. New image `kubedojo/linux`.
- **KCNA and KCSA tracks.** 43 and 41 multiple-choice questions with a lesson per domain. Practice
  shows the explanation for every option after answering; a mock exam is 30 questions in 45 minutes
  and reveals nothing until it is finished. These tracks need no Docker.
- **Road to Kubestronaut on the home page.** The recommended order Linux & Vim → KCNA → CKAD → CKA →
  KCSA → CKS with readiness per step and how many of the five mock exams are passed. Nothing is
  locked. Defined in `content/path.yaml`.
- Track kinds (`certification`, `skills`, `theory`) in `track.yaml`. The UI words every page by kind:
  a Linux track is never presented as a Kubernetes exam.
- `docs/curriculum-design.md`: the official sources behind the tracks, the order, and the limits.
- `shellcheck` on the Linux host.
- README, home page and curriculum design say who kubedojo is for: people who build and run software
  and want to stay in practice, not only exam candidates.
- A 50-second tour of the project, `docs/media/kubedojo-promo.mp4`, linked from the README.

### Changed

- A check's `on` is validated when content is loaded. A typo used to address a machine that does
  not exist.
- `kubedojo doctor` and `content test` ask for kind and the node image only when a selected track
  uses a cluster.
- Sessions of theory tracks do not count against `--max-sessions`.
- Guard checks score only once another check of the task has passed. An untouched task used to
  earn the points of its guards in an exam.
- A quick round is no longer taken for a full assessment: its report says "quick round", and the
  path on the home page counts and ticks full mock exams, exam simulations and skills assessments
  only. Exam records carry their number of questions and minutes.
- Hands-on practice and assessments work on a phone: below the `md` breakpoint task and terminal
  are two tabs, and the terminal keeps its connection and scrollback behind the other tab.
- The Linux track is worded as a skills track throughout: recommendations, the provisioning and
  grading log, the assessment workspace and the first line of its 40 task texts.
- Counts are singular or plural ("1 exercise", "2 questions") instead of "question(s)".
- A check that waits for its retry window says so after three seconds, with the seconds it has run.
- Check explanations are rendered as Markdown; expected values of several lines keep their line
  breaks instead of showing `\n`.

### Fixed

- Lessons no longer scroll sideways on a phone.
- The solution opens without a confirmation once every check has passed.
- Long domain titles wrap in the report instead of being cut off.

## [0.3.0] - 2026-11-19

### Added

- **CKAD track.** Certified Kubernetes Application Developer: 5 domains with a lesson each,
  22 questions, full exams of 16 tasks in 120 minutes.
- `podman` and `buildah` on the exam nodes, for building and saving container images.
- Browser tests for the terminal keyboard (`make e2e`).
- `content lint` warns about checks that probe with `kubectl run --rm`.
- CI runs the content proof for the CKAD track on pull requests and every night.
- `docs/adding-a-track.md`.

### Changed

- Terminal tab shortcuts on macOS are Ctrl+Option+T/W/←/→/1–9. Other platforms keep Alt+Shift.
- On macOS the Option key types characters again instead of acting as Meta. Use `Esc` followed by
  the key for Meta shortcuts such as word movement.
- `kubedojo doctor` rebuilds a sandbox image that an older kubedojo built, so new tools reach
  existing installations.
- `kubedojo doctor` pulls the sandbox images from `ghcr.io/mistral-valaise`, where releases publish
  them, instead of trying Docker Hub.
- CI on pull requests is faster: it proves only the questions a change touches, looks once at
  checks that are expected to fail (`content test --quick`), caches the sandbox images and no
  longer waits for cross-compiling. The nightly job still proves everything in full.
- CI uses `actions/upload-artifact@v7` (Node 24).

### Fixed

- German and other keyboard layouts that type `| @ [ ] { } \ ~` with Option could not enter these
  characters in the terminal on macOS; Option+7 showed `(arg: 7)` ([#5](https://github.com/Mistral-valaise/kubedojo/issues/5)).
- CKA `fix-broken-rollout`: the "Service answers" check failed at random.
- The log line "sandboxes run as pods" in hosted mode showed an empty image when the default
  image was used.
- CKA `failing-container-logs`: the hint pointed to `logs --previous`, which usually fails while the
  container waits in CrashLoopBackOff; the worked solution could write an empty answer.

## [0.2.0] - 2026-11-11

Kubernetes 1.37, click-to-copy and run buttons, question ratings, registry pull-through cache,
Killercoda export, refreshed README.

## [0.1.0] - 2026-11-11

First release: CKA and CKS tracks with 58 verified questions, practice and exam modes, local and
Helm deployment, OpenTelemetry observability.

[0.4.0]: https://github.com/Mistral-valaise/kubedojo/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/Mistral-valaise/kubedojo/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Mistral-valaise/kubedojo/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Mistral-valaise/kubedojo/releases/tag/v0.1.0
