### Fixed

- The `chat <model>` span starts on the clock that `gen_ai.response.time_to_first_chunk` and the
  duration metric are measured from. Before, the span started a little later, so on a very fast
  answer the time to the first chunk could be longer than the span itself.
