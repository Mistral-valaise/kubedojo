### Added

- The AI tutor becomes an **assistant**: a conversation per question instead of one answer, streamed
  while the model writes. It is described in [docs/assistant.md](docs/assistant.md). The UI for it
  follows; this release has the server side and its HTTP API.
- A second provider next to Claude: any server with the OpenAI chat-completions API, such as a local
  Ollama or LM Studio, vLLM, OpenAI or OpenRouter.
  `kubedojo up --ai --ai-provider openai --ai-base-url http://localhost:11434/v1 --ai-model llama3.2`
  runs the assistant on a local model, without a key and without anything leaving the machine.
- Flags `--ai`, `--ai-provider`, `--ai-model`, `--ai-base-url` and `--ai-trace-content` for `up` and
  `serve`, with the environment variables `KUBEDOJO_AI`, `KUBEDOJO_AI_PROVIDER`, `KUBEDOJO_AI_MODEL`,
  `KUBEDOJO_AI_BASE_URL`, `KUBEDOJO_AI_API_KEY` and `KUBEDOJO_AI_TRACE_CONTENT`. A flag wins over
  the environment, the environment over the settings file.
- On a local server the assistant can be configured while it runs: `GET` and `PUT /api/settings`,
  and `POST /api/settings/test` to try a configuration before saving it. The settings are stored in
  `settings.json` in the data directory, readable by the owner only. The API key is write-only: no
  endpoint, log line or error shows it. `kubedojo backup` does not copy the file.
- With every message the assistant sees more of the task: the hints and the solution the learner
  has opened, the learner's note, and for failing checks the output of fixed read-only commands
  (`kubectl get` and `describe` of the object a check names, the head of the file a check reads).
  The model itself runs nothing.
- The conversation of a question is stored and shown again when the question is reopened
  (`GET`, `DELETE /api/sessions/{id}/questions/{q}/assistant`). It is part of the database and of a
  backup.
- Limits for the assistant: 4000 characters per message, 10 messages per user and minute, 4 answers
  at once on the server and one per question.
- `GET /api/config` has `assistant: {enabled, configured, provider, model, writable}`.
- The base URL of a provider is checked before it is used and again when the connection is made:
  plain `http` only for loopback and private addresses, never link-local addresses (cloud metadata),
  no redirects.
- Helm: `assistant.enabled`, `assistant.provider`, `assistant.model`, `assistant.baseURL`,
  `assistant.existingSecret` and `assistant.traceContent`.

### Changed

- An answer of the assistant can take up to 180 seconds (the tutor gave up after 90).
- The Anthropic client is configured by kubedojo alone. `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`
  and `ANTHROPIC_BASE_URL` work as before; `ANTHROPIC_CUSTOM_HEADERS` is no longer picked up.
- A base URL for Claude from `ANTHROPIC_BASE_URL` has to pass the same check as every provider URL:
  `https`, or `http` to a loopback or private address.
- Helm: the key of the assistant is passed as `KUBEDOJO_AI_API_KEY` instead of `ANTHROPIC_API_KEY`.

### Deprecated

- `POST /api/sessions/{id}/questions/{q}/tutor`. It still answers `{"markdown": "..."}` and now
  sends `Deprecation: true`. Use `POST .../assistant`.
- The environment variable `KUBEDOJO_TUTOR_MODEL`. Use `KUBEDOJO_AI_MODEL` or `--ai-model`.
- The Helm values `tutor.existingSecret` and `tutor.model`. Use `assistant.existingSecret` and
  `assistant.model`.
