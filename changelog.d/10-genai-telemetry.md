### Added

- A message to the assistant is one trace, from the request to the model: `assistant.chat` for the
  answer, `assistant.context` for what kubedojo gathers first (with the `sandbox.exec` spans of the
  commands it reads), `chat <model>` for the call to the model and the HTTP request to the provider
  below it. `assistant.chat` names the conversation, the turn and how the answer ended
  (`kubedojo.assistant.result`).
- The call to the model follows the OpenTelemetry semantic conventions for generative AI
  (`open-telemetry/semantic-conventions-genai` v0.1.0, status: development): one `chat <model>`
  client span for every provider with `gen_ai.provider.name`, the requested model and the model that
  answered, the finish reason, the response id, the time to the first text and the token counts,
  cached tokens included.
- Metrics of the conventions: `gen_ai_client_inference_duration_seconds`,
  `gen_ai_client_inference_time_to_first_chunk_seconds`, the token counters
  `gen_ai_client_inference_usage_{input,output,cache_read_input,cache_write_input}_tokens_total` and
  the per-call histograms `gen_ai_client_inference_operation_{input,output}_tokens`. And kubedojo's
  own: `kubedojo_assistant_requests_total` (by `provider`, `phase`, `result`, `track`, `mode`; it
  also counts the messages kubedojo refuses before an answer starts) and
  `kubedojo_assistant_context_duration_seconds`. The samples of an answer carry its trace as an
  exemplar. No label holds a session, a user or text.
- Prompts and answers are recorded on the `chat` span only where `--ai-trace-content` allows it: by
  default for the bundled local observability stack and nowhere else. What is recorded has the
  structure the conventions define (`gen_ai.system_instructions`, `gen_ai.input.messages`,
  `gen_ai.output.messages`), is redacted like the output of a check and cut at 32 KiB per attribute.
- A model provider gets no trace context: the HTTP request to it is a span of the trace, and it is
  sent without `traceparent` and `baggage` headers.
- The `meta` event of an answer's stream carries `traceId` when the request is traced, so that a
  client can show the trace of an answer that failed.
- A Grafana dashboard, **kubedojo / Assistant**: messages by result, failures by kind, duration and
  time to the first text by provider and model with exemplars, tokens per hour and per message, the
  time to gather the context, refused messages, recent calls to the model and the log lines of the
  assistant.
- `docs/observability.md` has a section "The assistant" with the span tree, every attribute, the
  content rule, the token counts, the metrics and TraceQL queries to start from.

### Changed

- `usage.inputTokens` in the `done` event of an answer counts everything the model read, for every
  provider: with the Claude API it now includes the tokens read from and written to the prompt
  cache, which are still reported as `cacheReadTokens` and `cacheWriteTokens` as well.
- The provider name of a Mistral endpoint is `mistral` (it was `mistral_ai`), as the OpenTelemetry
  conventions now spell it. It shows in `GET /api/config`, in the stream's `meta` event and in
  telemetry.
