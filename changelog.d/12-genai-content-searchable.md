### Fixed

- The prompts and answers recorded on the `chat` span can be found in Tempo. They were structured
  values, which Tempo shows on a span but cannot search: a TraceQL query on
  `gen_ai.input.messages` found nothing. They are now JSON text in the same structure, which the
  GenAI conventions allow for such a backend, and `{ span.gen_ai.input.messages =~ ".*kubectl.*" }`
  finds a conversation by what was said in it. The bundled Tempo keeps them whole: it cut every
  attribute at 2 KiB, and now at 256 KiB. An existing local stack is recreated with the new setting
  on the next `kubedojo up --observability`; its data is kept.
- An assistant whose model reached its output limit before it wrote a word (a reasoning model such
  as `qwen3.5` can spend the whole limit on thinking) says so, instead of "The model returned no
  text."
