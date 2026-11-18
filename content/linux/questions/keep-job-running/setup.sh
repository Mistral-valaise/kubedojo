#!/usr/bin/env bash
# A collector that the learner has to start so that it outlives the login,
# and an import that somebody suspended and never continued.
set -euo pipefail
kd-fixture reset
# What the learner started in a login shell does not carry the marker reset
# looks for: end it by name. The names belong to this exercise alone.
pkill -KILL -x keepjob-collect 2>/dev/null || true
pkill -KILL -x keepjob-import 2>/dev/null || true

base=/opt/course/keep-job-running
rm -rf "$base"
mkdir -p "$base/bin"

cat > "$base/bin/keepjob-collect" <<'PY'
#!/usr/bin/python3
"""keepjob-collect: takes a sample every few seconds until it is told to stop.

Progress goes to standard output, warnings go to standard error.
"""
import os
import signal
import sys
import time

signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))
print(f"collector started pid={os.getpid()}", flush=True)
print("collector: warning: no upstream configured, keeping samples locally",
      file=sys.stderr, flush=True)
n = 0
while True:
    time.sleep(5)
    n += 1
    print(f"sample {n} taken", flush=True)
PY
cat > "$base/bin/keepjob-import" <<'PY'
#!/usr/bin/python3
"""keepjob-import: a long import. It only waits, so that it costs nothing."""
import signal
import sys

signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))
print("import running", flush=True)
while True:
    signal.pause()
PY
chmod 0755 "$base/bin/keepjob-collect" "$base/bin/keepjob-import"
chown -R candidate:candidate "$base"

kd-fixture start --user candidate import -- "$base/bin/keepjob-import"
pid=$(kd-fixture pid import)
# Suspend it only once it runs its own code: its name says so.
for _ in $(seq 100); do
  [ "$(ps -o comm= -p "$pid")" = keepjob-import ] && grep -q 'import running' "$(kd-fixture log import)" && break
  sleep 0.1
done
kill -STOP "$pid"
ok=
for _ in $(seq 100); do
  case "$(ps -o stat= -p "$pid")" in T*) ok=1; break ;; esac
  sleep 0.1
done
[ -n "$ok" ] || { echo "the import could not be suspended" >&2; exit 1; }
