# Changelog

Notable changes per release. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

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

[0.3.0]: https://github.com/Mistral-valaise/kubedojo/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Mistral-valaise/kubedojo/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Mistral-valaise/kubedojo/releases/tag/v0.1.0
