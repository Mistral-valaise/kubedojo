### Added

- The home page starts with one clear next step: "Continue with <track>"
  leads back to an open session or to the track you have come furthest in,
  and "Browse all tracks" jumps to the track list. The learning path sits
  next to it as a compact card with the readiness of every step.
- Every page has a footer with the version, a link to the source on GitHub
  and the author. The practice and exam views keep their full height for the
  terminal and show the same three items behind an "About kubedojo" button in
  their header.
- An address that leads nowhere shows a "Page not found" page with the way
  back, instead of the start page. A page that fails to render shows a message
  with a reload button instead of an empty window.
- Pages show placeholders in the shape of their content while they load, and
  say so when there is nothing to show.

### Changed

- The web interface has a refreshed look on one set of design tokens: revised
  colours for the dark and the light theme with readable contrast everywhere
  (at least 4.5:1 for text), one type scale with 12 px as its smallest size,
  larger touch targets, and one look for cards, buttons and form fields. The
  terminal stays dark in both themes.
- The fonts (Inter and JetBrains Mono) are shipped with kubedojo and served
  from its own address. No request leaves your machine for a font, and the
  interface looks the same without internet access. The terminal waits a
  moment for its font before it opens, so that its columns line up.
- Check results show a failed check highlighted with what was expected and
  what was found; hints and the solution sit in one row below them.
- Dialogs keep the keyboard focus inside while they are open and return it to
  where it was when they close.
- Inline code in task texts is still copied with a click, and is no longer a
  stop of the Tab key: a task with dozens of snippets had dozens of stops
  before its first real control. The copy and run buttons of a code block
  are always visible on a touch screen.

### Fixed

- Errors that were swallowed are now shown, each with a way to try again: a
  question of a running exam or of a report that could not be loaded (it
  showed a spinner forever), a note or a flag that could not be saved during
  an exam, the progress, the learning path and the sessions on the home page,
  and a session that could not be ended there.
- The home page fetched the list of tracks twice.
- On a phone the progress of a domain on the track page no longer wraps
  awkwardly, long question titles are no longer cut off, and the terminal of
  an exam report takes a share of the screen that fits its height.
- Accessibility: a "Skip to main content" link on every view; the practice and
  exam views have a main landmark; the divider between task and terminal has
  a name and a value; the star rating is a radio group that the arrow keys
  move through; the theme button says whether the dark theme is on; the
  question buttons of an exam are named, and a flagged question is marked by
  a flag and not by colour alone; hints and tutor answers are announced when
  they appear; all motion stops when the system asks for reduced motion.
