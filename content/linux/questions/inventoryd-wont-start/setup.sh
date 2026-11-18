#!/usr/bin/env bash
# A small service with an init script, as on a host without systemd. It does
# not start for two reasons, one behind the other: /etc/default points to a
# configuration file that does not exist, and its port is held by an instance
# of the previous release that somebody started by hand.
set -euo pipefail
kd-fixture reset

base=/opt/course/inventoryd-wont-start
svc=inventoryd
port=22210

# The service the learner (or a solution) started does not carry the marker
# reset looks for: stop it by name. Both names belong to this exercise alone.
pkill -KILL -x inventoryd 2>/dev/null || true
pkill -KILL -x inventoryd-old 2>/dev/null || true
for _ in $(seq 100); do
  (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null || break
  sleep 0.1
done
rm -rf /etc/inventoryd /etc/default/inventoryd /etc/init.d/inventoryd /run/inventoryd.pid /var/log/inventoryd.log

if id "$svc" >/dev/null 2>&1; then
  pkill -KILL -u "$svc" 2>/dev/null || true
  userdel "$svc"
fi
if getent group "$svc" >/dev/null; then groupdel "$svc"; fi
useradd --system --no-create-home --user-group --shell /usr/sbin/nologin "$svc"

rm -rf "$base"
mkdir -p "$base/bin" "$base/data" "$base/old-release" /etc/inventoryd

cat > "$base/bin/inventoryd" <<'PY'
#!/usr/bin/python3
"""inventoryd: serves the inventory over HTTP.

  inventoryd --config <file>

The file has lines key=value: port (TCP port on 127.0.0.1) and data (a JSON
file with the items). GET /healthz and GET /items.
"""
import argparse
import http.server
import json
import signal
import sys

RELEASE = "2.4"


def die(code, message):
    print(f"inventoryd: {message}", file=sys.stderr, flush=True)
    sys.exit(code)


parser = argparse.ArgumentParser()
parser.add_argument("--config", required=True)
args = parser.parse_args()

conf = {}
try:
    with open(args.config) as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                key, value = line.split("=", 1)
                conf[key.strip()] = value.strip()
except OSError as e:
    die(2, f"cannot read configuration {args.config}: {e.strerror}")

try:
    port = int(conf["port"])
    with open(conf["data"]) as f:
        items = json.load(f)
except KeyError as e:
    die(2, f"{args.config}: setting {e} is missing")
except (OSError, ValueError) as e:
    die(2, f"cannot load data file {conf.get('data')}: {e}")


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/healthz":
            self.reply(200, "text/plain", f"inventoryd {RELEASE} ok\n")
        elif self.path == "/items":
            self.reply(200, "application/json", json.dumps(items) + "\n")
        else:
            self.reply(404, "text/plain", "not found\n")

    def reply(self, code, ctype, body):
        data = body.encode()
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, fmt, *a):
        print("inventoryd: " + fmt % a, file=sys.stderr, flush=True)


try:
    server = http.server.HTTPServer(("127.0.0.1", port), Handler)
except OSError as e:
    die(3, f"cannot listen on 127.0.0.1:{port}: {e.strerror}")

signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))
print(f"inventoryd: release {RELEASE} listening on 127.0.0.1:{port}, {len(items)} items",
      file=sys.stderr, flush=True)
server.serve_forever()
PY

cat > "$base/old-release/inventoryd-old" <<'PY'
#!/usr/bin/python3
"""Release 1.9 of inventoryd, started by hand for a test and forgotten."""
import http.server
import signal
import sys


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        data = b"inventoryd 1.9 (test instance)\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, fmt, *a):
        pass


signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))
http.server.HTTPServer(("127.0.0.1", int(sys.argv[1])), Handler).serve_forever()
PY
chmod 0755 "$base/bin/inventoryd" "$base/old-release/inventoryd-old"

cat > "$base/data/items.json" <<'JSON'
[{"sku": "KD-1001", "name": "torque wrench", "stock": 14},
 {"sku": "KD-1002", "name": "feeler gauge", "stock": 3},
 {"sku": "KD-1003", "name": "dial indicator", "stock": 0},
 {"sku": "KD-1004", "name": "thread gauge", "stock": 27}]
JSON

cat > /etc/inventoryd/inventoryd.conf <<CONF
# inventoryd settings. Clients are configured for this port.
port=$port
data=$base/data/items.json
CONF

# The fault: after the package was renamed, the path still has the old name.
cat > /etc/default/inventoryd <<'CONF'
# Defaults for the inventoryd init script, read by /etc/init.d/inventoryd.
CONFIG=/etc/inventory/inventory.conf
CONF

cat > /etc/init.d/inventoryd <<'INIT'
#!/bin/sh
### BEGIN INIT INFO
# Provides:          inventoryd
# Required-Start:    $remote_fs $network
# Required-Stop:     $remote_fs $network
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: inventory service of the kubedojo exercise inventoryd-wont-start
### END INIT INFO

NAME=inventoryd
DAEMON=/opt/course/inventoryd-wont-start/bin/inventoryd
PIDFILE=/run/inventoryd.pid
LOGFILE=/var/log/inventoryd.log
RUNAS=inventoryd
CONFIG=/etc/inventoryd/inventoryd.conf

[ -r /etc/default/inventoryd ] && . /etc/default/inventoryd

running() {
  [ -s "$PIDFILE" ] || return 1
  pid=$(cat "$PIDFILE")
  # Not kill -0: status is also asked by users who may not signal the daemon.
  [ -d "/proc/$pid" ] && [ "$(ps -o comm= -p "$pid")" = "$NAME" ]
}

do_start() {
  if running; then echo "$NAME is already running (pid $(cat "$PIDFILE"))"; return 0; fi
  rm -f "$PIDFILE"
  touch "$LOGFILE"; chmod 0644 "$LOGFILE"
  before=$(wc -l < "$LOGFILE")
  echo "$(date '+%F %T') init: starting $NAME with --config $CONFIG" >> "$LOGFILE"
  start-stop-daemon --start --quiet --background --make-pidfile --pidfile "$PIDFILE" \
    --chuid "$RUNAS" --output "$LOGFILE" --startas "$DAEMON" -- --config "$CONFIG"
  # The daemon either stays or gives up within a moment: wait for one of the two.
  i=0
  while [ "$i" -lt 100 ]; do
    if ! running; then break; fi
    if tail -n "+$((before + 1))" "$LOGFILE" | grep -q 'listening on'; then
      echo "Starting $NAME: ok (pid $(cat "$PIDFILE"))"; return 0
    fi
    i=$((i + 1)); sleep 0.1
  done
  rm -f "$PIDFILE"
  echo "Starting $NAME: failed. See $LOGFILE" >&2
  return 1
}

do_stop() {
  if ! running; then echo "$NAME is not running"; rm -f "$PIDFILE"; return 0; fi
  start-stop-daemon --stop --quiet --retry TERM/5/KILL/2 --pidfile "$PIDFILE" --name "$NAME"
  rm -f "$PIDFILE"
  echo "Stopping $NAME: ok"
}

case "$1" in
  start)   do_start ;;
  stop)    do_stop ;;
  restart) do_stop; do_start ;;
  status)
    if running; then echo "$NAME is running (pid $(cat "$PIDFILE"))"; exit 0; fi
    echo "$NAME is not running"; exit 3 ;;
  *) echo "Usage: service $NAME {start|stop|restart|status}" >&2; exit 2 ;;
esac
INIT
chmod 0755 /etc/init.d/inventoryd

install -d -m 0700 "$base/.ref"
sha256sum "$base/bin/inventoryd" "$base/data/items.json" > "$base/.ref/manifest"

chown candidate:candidate "$base"
chown -R candidate:candidate "$base/old-release"

# The forgotten test instance holds the port. It runs as candidate, the way
# it would if the learner's colleague had started it from a login shell.
kd-fixture start --user candidate old -- "$base/old-release/inventoryd-old" "$port"
ok=
for _ in $(seq 100); do
  if (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then ok=1; break; fi
  sleep 0.1
done
[ -n "$ok" ] || { echo "the old instance did not come up" >&2; exit 1; }
