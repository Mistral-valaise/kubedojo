#!/usr/bin/env bash
# Three queue workers that look the same in `ps`, each listening on its own
# port. The one on port 22110 is the leftover that has to go.
set -euo pipefail
kd-fixture reset
# A worker the learner started by hand does not carry the marker reset looks for.
pkill -KILL -x port-hog-worker 2>/dev/null || true
# The ports are free again once the old workers are really gone.
for port in 22110 22111 22112; do
  for _ in $(seq 100); do
    (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null || break
    sleep 0.1
  done
done

base=/opt/course/port-hog
rm -rf "$base"
mkdir -p "$base/bin" "$base/state"

cat > "$base/bin/port-hog-worker" <<'PY'
#!/usr/bin/python3
"""port-hog-worker: serves one queue on a TCP port.

On SIGTERM, SIGINT, SIGHUP or SIGQUIT it finishes cleanly and leaves a note in
the state directory. SIGKILL gives it no chance to do so.
"""
import argparse
import os
import signal
import socket
import sys

STATE = "/opt/course/port-hog/state"

parser = argparse.ArgumentParser()
parser.add_argument("--queue", required=True)
args = parser.parse_args()
port = int(os.environ["PORT_HOG_LISTEN"])

srv = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
srv.bind(("127.0.0.1", port))
srv.listen(8)


def shutdown(signum, _frame):
    srv.close()
    with open(f"{STATE}/worker-{os.getpid()}.exit", "w") as f:
        f.write(f"clean shutdown after {signal.Signals(signum).name}\n")
    sys.exit(0)


for s in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP, signal.SIGQUIT):
    signal.signal(s, shutdown)

print(f"worker for queue {args.queue} ready", flush=True)
while True:
    conn, _ = srv.accept()
    with conn:
        conn.sendall(f"queue {args.queue}\n".encode())
PY
chmod 0755 "$base/bin/port-hog-worker"
chown -R candidate:candidate "$base"

# start <fixture> <port> <queue>. The port is not on the command line, so the
# three differ only in the queue: the port has to be looked up.
start() {
  kd-fixture start --user candidate "$1" -- env "PORT_HOG_LISTEN=$2" "$base/bin/port-hog-worker" --queue "$3"
}
start orders  22111 orders
start billing 22110 billing
start mail    22112 mail

# Return only when all three listen.
for port in 22110 22111 22112; do
  ok=
  for _ in $(seq 100); do
    if (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then ok=1; break; fi
    sleep 0.1
  done
  [ -n "$ok" ] || { echo "the worker on port $port did not come up" >&2; exit 1; }
done
