# Credits for the videos

Both videos show kubedojo itself, recorded key by key from a local run of this repository.

## `kubedojo-promo.mp4`: the tour

A manifest that looks right is applied, three Pods end in `ImagePullBackOff`, and the camera pulls
back one step at a time: the task and its failed check, the assistant, all checks green, the track,
the learning loop, the path, one machine and a team's cluster.

- Recorded: the practice workspace, the assistant (answering on a local model through Ollama), a
  track, a lesson, an exam simulation with its report and the home page. The manifest is the one from
  the question's own `setup.sh`; the video does not claim that a particular tool wrote it.
- Drawn: the pictures of the learning loop, of one machine and of a team's cluster, after the
  diagrams in the README. The Grafana panel is `docs/images/grafana.png`.

## `kubedojo-architecture.mp4`: how it is built

The architecture in the levels of the [C4 model](https://c4model.com): system context, containers,
components and code, then the terminal, the assistant, a trace and the hosted mode.

- Recorded: the click on "Check my solution" at the start, and the texts that are shown as output:
  `kubedojo doctor`, the names of the containers of a session, and what runs inside a node.
- Drawn: every diagram. Their names and sentences come from `docs/architecture.md`, `docs/deployment.md`,
  `docs/observability.md`, `docs/assistant.md` and the code. The `Driver` interface is shown without
  its parameter types, which the video says, and the `run` command of the check is cut short with an
  ellipsis.

## Music

"Lofi Hip Hop Upbeat" by raspberrymusic, licensed under
[Creative Commons Attribution 4.0](https://creativecommons.org/licenses/by/4.0/).
Source: <https://commons.wikimedia.org/wiki/File:Raspberrymusic_-_Lofi_Hip_Hop_Upbeat.ogg>.
Both videos are longer than the track. Its sections are played in another order and its eight-bar
loop more than once, joined on bar lines, and the volume is lowered.

## Sound effects

Typing, key press, click, whoosh, pop and the bass impact come from the sound library bundled with
the [HyperFrames](https://hyperframes.heygen.com) command line tool, which also rendered the videos.
