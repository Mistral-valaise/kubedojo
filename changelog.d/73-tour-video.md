### Added

- A second video, `docs/media/kubedojo-architecture.mp4`, explains how kubedojo is built, in the levels
  of the C4 model: system context, containers, components and code, then the terminal, the assistant,
  a trace and the hosted mode. It is linked from the README and from `docs/architecture.md`.

### Changed

- The tour video `docs/media/kubedojo-promo.mp4` is new and shows 0.5. A manifest that looks right
  is applied and three Pods fail; from there the camera pulls back one step at a time: the failed
  check with its reason, the assistant answering on a local model, the track, the learning loop, the
  path, one machine and a team's cluster. The music and sound effects of both videos are credited in
  `docs/media/CREDITS.md`.
- The README lists `kubedojo backup` and `kubedojo restore`, and the package table of
  `docs/architecture.md` names every command of the CLI.

### Fixed

- The assistant's panel no longer shows the empty conversation ("Stuck, or curious why something
  works?") for a moment before the stored conversation of a question appears. It shows the loading
  placeholder from its first render.
