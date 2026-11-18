#!/usr/bin/env bash
# Two instances of one exporter. "nightly" hangs and ignores the polite
# signals; "hourly" is healthy but still runs with the configuration it read
# at start, which has changed on disk since.
set -euo pipefail
kd-fixture reset
# An exporter the learner started by hand does not carry the marker reset looks for.
pkill -KILL -x stuck-exporter 2>/dev/null || true

base=/opt/course/stuck-exporter
rm -rf "$base"
mkdir -p "$base/bin" "$base/conf" "$base/run"

cat > "$base/bin/stuck-exporter" <<'PY'
#!/usr/bin/python3
"""stuck-exporter: exports records for one job. See ../README for its signals."""
import argparse
import os
import signal
import sys

BASE = "/opt/course/stuck-exporter"

parser = argparse.ArgumentParser()
parser.add_argument("--job", required=True)
args = parser.parse_args()
job = args.job
lock = f"{BASE}/run/{job}.lock"
status = f"{BASE}/run/{job}.status"
reloads = 0


def read_conf():
    conf = {}
    with open(f"{BASE}/conf/{job}.conf") as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                key, value = line.split("=", 1)
                conf[key.strip()] = value.strip()
    return conf


def write_status(state):
    conf = read_conf() if state != "hung" else {"level": "info"}
    tmp = status + ".tmp"
    with open(tmp, "w") as f:
        f.write(f"job={job}\npid={os.getpid()}\nstate={state}\n"
                f"level={conf.get('level', 'info')}\nreloads={reloads}\n")
    os.replace(tmp, status)


def reload(_signum, _frame):
    global reloads
    reloads += 1
    write_status("running")


def stop(_signum, _frame):
    write_status("stopped")
    try:
        os.unlink(lock)
    except FileNotFoundError:
        pass
    sys.exit(0)


with open(lock, "w") as f:
    f.write(f"{os.getpid()}\n")

if os.environ.get("STUCK_EXPORTER_FAULT") == "hang":
    # The bug this exercise is about: stuck, and deaf to every polite signal.
    for s in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP, signal.SIGQUIT,
              signal.SIGUSR1, signal.SIGUSR2):
        signal.signal(s, signal.SIG_IGN)
    write_status("hung")
else:
    signal.signal(signal.SIGHUP, reload)
    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    write_status("running")

while True:
    signal.pause()
PY
chmod 0755 "$base/bin/stuck-exporter"

cat > "$base/README" <<'TXT'
stuck-exporter --job <name>

  conf/<name>.conf     settings, read at start
  run/<name>.status    what the running process is doing, and with which settings
  run/<name>.lock      exists while a job runs; a job does not start while its lock exists

Signals
  SIGHUP    read conf/<name>.conf again and keep running
  SIGTERM   finish the current batch, remove the lock, exit
  SIGINT    the same as SIGTERM
TXT
printf '%s\n' '# nightly full export' 'level=info' 'target=/srv/export/full' > "$base/conf/nightly.conf"
printf '%s\n' '# hourly delta export' 'level=info' 'target=/srv/export/delta' > "$base/conf/hourly.conf"
chown -R candidate:candidate "$base"

kd-fixture start --user candidate nightly -- env STUCK_EXPORTER_FAULT=hang "$base/bin/stuck-exporter" --job nightly
kd-fixture start --user candidate hourly -- "$base/bin/stuck-exporter" --job hourly

# Both have written their status: their signal handlers are in place.
for job in nightly hourly; do
  ok=
  for _ in $(seq 100); do
    if [ -s "$base/run/$job.status" ]; then ok=1; break; fi
    sleep 0.1
  done
  [ -n "$ok" ] || { echo "the $job exporter did not come up" >&2; exit 1; }
done

# Now the configuration changes on disk; the running process does not know.
printf '%s\n' '# hourly delta export' 'level=debug' 'target=/srv/export/delta' > "$base/conf/hourly.conf"
chown candidate:candidate "$base/conf/hourly.conf"
