### Changed

- The API limits the size of request bodies: 64 KiB to create a session, 16 KiB to change a
  question (note, flag, answer), 8 KiB to grade, rate or ask the tutor. A larger body is rejected
  with `400`. The requests of the UI are far below these limits.
