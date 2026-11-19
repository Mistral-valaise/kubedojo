#!/usr/bin/env bash
# An order service that does not start since last night's change. Two faults,
# and the log only ever shows the first one the service runs into:
#   1. a typo in data_dir (the change also raised max_orders, which is wanted),
#   2. the rotated API token was written by root with mode 600, so the service
#      account cannot read it.
set -euo pipefail

id=broken-after-deploy
user=broken-after-deploy
base=/opt/course/$id

kd-fixture reset
# Also what a learner started by hand (it carries no KD_QUESTION).
fuser -s -k -KILL 23180/tcp 2> /dev/null || true
if id "$user" > /dev/null 2>&1; then
  pkill -KILL -u "$user" 2> /dev/null || true
else
  useradd --system --no-create-home --shell /usr/sbin/nologin -c "order service ($id)" "$user"
fi

rm -rf "$base"
mkdir -p "$base/bin" "$base/etc" "$base/var/data" "$base/var/log" "$base/.state"

cat > "$base/bin/ordersd.py" <<'PY'
#!/usr/bin/env python3
"""ordersd: a small order service. Usage: ordersd.py <config file>"""
import datetime
import errno
import glob
import http.server
import json
import os
import pwd
import signal
import sys
import time

VERSION = "3.2.0"
conf_path = sys.argv[1] if len(sys.argv) > 1 else "/opt/course/broken-after-deploy/etc/orders.conf"
log_file = None


def log(level, msg):
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    line = "%s %-5s %s\n" % (stamp, level, msg)
    sys.stdout.write(line)
    sys.stdout.flush()
    if log_file:
        with open(log_file, "a") as f:
            f.write(line)


def fail(msg):
    log("ERROR", "startup failed: " + msg)
    sys.exit(1)


conf = {}
try:
    with open(conf_path) as f:
        for raw in f:
            line = raw.split("#", 1)[0].strip()
            if line:
                key, _, value = line.partition("=")
                conf[key.strip()] = value.strip()
except OSError as e:
    sys.exit("ordersd: cannot read %s: %s" % (conf_path, e.strerror))

who = pwd.getpwuid(os.geteuid()).pw_name
try:
    with open(conf.get("log_file", ""), "a"):
        pass
    log_file = conf["log_file"]
except (OSError, KeyError) as e:
    sys.exit("ordersd: cannot write log_file %r as %s: %s" % (conf.get("log_file"), who, e))

log("INFO", "ordersd %s starting, config %s, user %s" % (VERSION, conf_path, who))
for key in ("listen_address", "port", "data_dir", "max_orders", "token_file"):
    if key not in conf:
        fail("%s is not set in %s" % (key, conf_path))
try:
    port, max_orders = int(conf["port"]), int(conf["max_orders"])
except ValueError as e:
    fail("not a number: %s" % e)

if not os.path.isdir(conf["data_dir"]):
    fail("data_dir %s is not a directory" % conf["data_dir"])
orders = sorted(glob.glob(os.path.join(conf["data_dir"], "*.json")))
log("INFO", "loaded %d orders from %s, max_orders %d" % (len(orders), conf["data_dir"], max_orders))

try:
    with open(conf["token_file"]) as f:
        token = f.read().strip()
except OSError as e:
    fail("cannot read token_file %s as user %s: %s" % (conf["token_file"], who, e.strerror))
if not token:
    fail("token_file %s is empty" % conf["token_file"])
log("INFO", "api token loaded from %s" % conf["token_file"])


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health":
            status, body = 200, {"status": "ok", "orders_loaded": len(orders), "max_orders": max_orders}
        else:
            status, body = 404, {"error": "not found"}
        data = (json.dumps(body) + "\n").encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, *args):
        pass


server = None
for _ in range(50):
    try:
        server = http.server.ThreadingHTTPServer((conf["listen_address"], port), Handler)
        break
    except OSError as e:
        if e.errno != errno.EADDRINUSE:
            fail("cannot listen on %s:%d: %s" % (conf["listen_address"], port, e.strerror))
        time.sleep(0.1)
if server is None:
    fail("cannot listen on %s:%d: address already in use" % (conf["listen_address"], port))


def stop(*_):
    log("INFO", "received SIGTERM, shutting down")
    os._exit(0)


signal.signal(signal.SIGTERM, stop)
log("INFO", "listening on %s:%d" % (conf["listen_address"], port))
server.serve_forever()
PY

cat > "$base/orderctl" <<'SH'
#!/usr/bin/env bash
# orderctl start|stop|restart|status: the "service manager" of ordersd on this
# host without systemd. The service runs as the account broken-after-deploy.
set -euo pipefail
export KD_QUESTION=broken-after-deploy
base=/opt/course/broken-after-deploy
user=broken-after-deploy

[ "$(id -u)" = 0 ] || { echo "orderctl: must run as root: sudo $0 ${1:-}" >&2; exit 1; }

pid() { kd-fixture pid orders 2> /dev/null; }
running() { p=$(pid) && kill -0 "$p" 2> /dev/null; }

start() {
  if kd-fixture start --user "$user" orders -- python3 "$base/bin/ordersd.py" "$base/etc/orders.conf" 2> /dev/null; then
    # Started means: it accepts connections, or it gave up.
    for _ in $(seq 100); do
      running || break
      # ss -e names the owner of a socket (-p cannot look into the processes
      # of other users in a container).
      if ss -Hltne | grep -q "uid:$(id -u "$user") "; then echo "ordersd started (pid $(pid))"; return 0; fi
      sleep 0.1
    done
  fi
  echo "orderctl: ordersd failed to start; see $base/var/log/orders.log" >&2
  return 1
}

stop() {
  if running; then
    p=$(pid)
    kill "$p"
    for _ in $(seq 50); do kill -0 "$p" 2> /dev/null || break; sleep 0.1; done
    echo "ordersd stopped"
  else
    echo "ordersd is not running"
  fi
}

case "${1:-}" in
  start)   start ;;
  stop)    stop ;;
  restart) stop; start ;;
  status)  if running; then echo "ordersd is running (pid $(pid))"; else echo "ordersd is not running"; exit 3; fi ;;
  *) echo "usage: sudo $0 start|stop|restart|status" >&2; exit 2 ;;
esac
SH
chmod 755 "$base/orderctl" "$base/bin/ordersd.py"

# The configuration before the change ...
cat > "$base/etc/orders.conf.bak" <<CONF
# ordersd configuration
listen_address = 127.0.0.1
port = 23180

data_dir = $base/var/data
max_orders = 100

token_file = $base/etc/api.token
log_file = $base/var/log/orders.log
CONF
# ... and after it: max_orders raised as planned, and a slip in data_dir.
sed -e 's/^max_orders = 100$/max_orders = 500/' -e 's|/var/data$|/var/dta|' \
  "$base/etc/orders.conf.bak" > "$base/etc/orders.conf"

cat > "$base/CHANGE-4711.txt" <<'TXT'
CHANGE-4711  (applied 2026-11-18 02:10 UTC by the deployment job, as root)

  1. Raise max_orders from 100 to 500 in etc/orders.conf for the sale on Friday.
     The previous file was kept as etc/orders.conf.bak.
  2. Rotate the API token: write a new etc/api.token.
  3. Restart ordersd.

Result: step 3 failed. ordersd has been down since.
TXT

for n in 1001 1002 1003; do
  printf '{"order": "ORD-%s", "items": %s, "state": "open"}\n' "$n" "$((n % 7 + 1))" > "$base/var/data/ORD-$n.json"
done

cat > "$base/var/log/orders.log" <<LOG
2026-11-17T06:00:02Z INFO  ordersd 3.2.0 starting, config $base/etc/orders.conf, user $user
2026-11-17T06:00:02Z INFO  loaded 3 orders from $base/var/data, max_orders 100
2026-11-17T06:00:02Z INFO  api token loaded from $base/etc/api.token
2026-11-17T06:00:02Z INFO  listening on 127.0.0.1:23180
2026-11-17T14:22:51Z WARN  order ORD-1002: stock low for item 4471
2026-11-17T19:03:40Z WARN  order ORD-1003: payment pending for 3600s
2026-11-18T02:10:11Z INFO  received SIGTERM, shutting down
2026-11-18T02:10:14Z INFO  ordersd 3.2.0 starting, config $base/etc/orders.conf, user $user
2026-11-18T02:10:14Z ERROR startup failed: data_dir $base/var/dta is not a directory
2026-11-18T02:10:44Z INFO  ordersd 3.2.0 starting, config $base/etc/orders.conf, user $user
2026-11-18T02:10:44Z ERROR startup failed: data_dir $base/var/dta is not a directory
LOG

# The rotated token: written by root, for root only. That is the second fault.
printf 'tok_live_%s\n' "$(head -c 18 /dev/urandom | od -An -tx1 | tr -d ' \n')" > "$base/etc/api.token"

chown -R candidate:candidate "$base"
chown root:root "$base/orderctl" "$base/bin/ordersd.py" "$base/etc/api.token" "$base/CHANGE-4711.txt"
chmod 600 "$base/etc/api.token"
# The service writes its own log.
chown -R "$user:$user" "$base/var/log"

sha256sum "$base/etc/api.token" > "$base/.state/token.sha256"
head -n 11 "$base/var/log/orders.log" | sha256sum > "$base/.state/log-head.sha256"
chown -R root:root "$base/.state"
chmod 700 "$base/.state"
