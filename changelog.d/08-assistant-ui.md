### Added

- The assistant has its place in the UI: a column next to the task and the terminal, opened with the
  **Assistant** button in the header of the practice view. On a medium screen it takes the place of
  the task while the terminal stays in view; on a phone it is a tab of its own. It is a conversation:
  answers appear while the model writes them, can be stopped and asked for again, and are there again
  when the question is reopened. See [docs/assistant.md](docs/assistant.md#in-the-ui).
- **Explain what went wrong** on a failed check opens the assistant and asks about the result as it
  stands. The review of a wrong answer to a multiple-choice question has **Explain**, and every
  question on the report of an exam has **Ask the assistant**.
- Above an answer the assistant says what kubedojo looked at for it: the task and the check results,
  the snapshot of the cluster or the host, the failing checks. Commands in an answer have the copy and
  run buttons of a task text.
- A **Settings** page for the assistant: switch, provider, base URL with presets for Ollama,
  LM Studio, OpenAI and OpenRouter, model and API key, with **Test connection** before saving. The key
  is never shown again once it is stored. Values that a flag or an environment variable sets are
  shown locked with the name of what sets them. On a hosted server the page is read-only. It is
  linked from the header of every page.
- When the assistant cannot answer it says why in a sentence and offers what helps: to try again, with
  the seconds to wait after a rate limit, or the way to the settings.

### Changed

- With the assistant open, the task and the terminal keep a least width; just above 1100 px the
  assistant gives way first, so that the terminal is not squeezed.
- The report of an exam arranges its two columns by the room it has, not by the width of the window.

### Removed

- The "AI tutor" box under the hints, with its single answer. The assistant replaces it.

### Fixed

- In the development build a dialog closed itself the moment it opened (React's strict mode ran its
  effect twice). Release builds were not affected.
- The headline of the check results no longer breaks into three lines in a narrow column: the points
  move below it.

### Security

- An answer of a model never loads an image: the alternative text is shown in its place. The address
  of an image could carry data out of the page; the Content-Security-Policy blocks such a request as
  well.
