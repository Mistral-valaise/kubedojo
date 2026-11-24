### Fixed

- Dashboards and alerts count the first event of a series. A counter that appeared with its first
  event (the first check of a question, the first message to the assistant) started at 1, and
  `increase()` and `rate()` did not see that first event: with few events, panels such as "Graded
  questions" showed too little. The bundled Prometheus now writes a zero at the start of a new
  series. An existing local stack is recreated with the new setting on the next
  `kubedojo up --observability`; its data is kept.
