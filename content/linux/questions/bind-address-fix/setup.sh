#!/usr/bin/env bash
# A metrics exporter with two listeners, both bound to the loopback address:
# local tests work, the scraper on the other machine gets "connection refused".
set -euo pipefail

id=bind-address-fix
base=/opt/course/$id

kd-fixture reset
# Also what a learner started by hand on these ports (it carries no KD_QUESTION).
for port in 23080 23081; do
  fuser -s -k -KILL "$port/tcp" 2> /dev/null || true
done

rm -rf "$base"
mkdir -p "$base/.state"

cat > "$base/exporter.conf" <<'CONF'
# Configuration of the metrics exporter. Read once, when the exporter starts.
#
# Metrics endpoint: scraped by the monitoring server over the network.
listen_address = 127.0.0.1
listen_port = 23080

# Admin endpoint (/-/reload, /-/quit): for operators on this host only.
admin_address = 127.0.0.1
admin_port = 23081
CONF

cat > "$base/exporter.py" <<'PY'
#!/usr/bin/env python3
"""Metrics exporter: /metrics on the metrics endpoint, /-/reload on the admin endpoint."""
import errno
import http.server
import sys
import threading
import time


def read_config(path):
    conf = {}
    with open(path) as f:
        for line in f:
            line = line.split("#", 1)[0].strip()
            if not line:
                continue
            if "=" not in line:
                sys.exit(f"exporter: {path}: cannot parse line: {line!r}")
            key, value = line.split("=", 1)
            conf[key.strip()] = value.strip()
    for key in ("listen_address", "listen_port", "admin_address", "admin_port"):
        if key not in conf:
            sys.exit(f"exporter: {path}: {key} is not set")
    return conf


def handler(routes):
    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            status, body = routes.get(self.path, (404, "not found\n"))
            data = body.encode()
            self.send_response(status)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def log_message(self, fmt, *args):
            print(f"{self.client_address[0]} {fmt % args}", flush=True)

    return Handler


def listen(address, port, routes):
    # IPv4 only. A port freed a moment ago by a restart may be busy for an instant.
    for _ in range(50):
        try:
            return http.server.ThreadingHTTPServer((address, int(port)), handler(routes))
        except OSError as e:
            if e.errno != errno.EADDRINUSE:
                sys.exit(f"exporter: cannot listen on {address}:{port}: {e}")
            time.sleep(0.1)
    sys.exit(f"exporter: cannot listen on {address}:{port}: address already in use")


conf = read_config(sys.argv[1] if len(sys.argv) > 1 else "exporter.conf")
metrics = listen(conf["listen_address"], conf["listen_port"], {
    "/metrics": (200, 'exporter_up 1\nexporter_build_info{version="1.4.2"} 1\n'),
})
admin = listen(conf["admin_address"], conf["admin_port"], {
    "/-/reload": (200, "the configuration is only read at start: restart the exporter\n"),
    "/-/quit": (403, "disabled\n"),
})
print(f"exporter: metrics on {conf['listen_address']}:{conf['listen_port']}, "
      f"admin on {conf['admin_address']}:{conf['admin_port']}", flush=True)
threading.Thread(target=admin.serve_forever, daemon=True).start()
metrics.serve_forever()
PY

cat > "$base/exporterctl" <<'SH'
#!/usr/bin/env bash
# exporterctl start|stop|restart|status: the "service manager" of the exporter
# on this host without systemd.
set -euo pipefail
export KD_QUESTION=bind-address-fix
base=/opt/course/bind-address-fix

[ "$(id -u)" = 0 ] || { echo "exporterctl: must run as root: sudo $0 ${1:-}" >&2; exit 1; }

pid() { kd-fixture pid exporter 2> /dev/null; }
running() { p=$(pid) && kill -0 "$p" 2> /dev/null; }
sockets() { ss -Hltnp | grep -c "pid=$(pid)," || true; }

start() {
  kd-fixture start exporter -- python3 "$base/exporter.py" "$base/exporter.conf"
  # Started means: both endpoints accept connections, or the process gave up.
  for _ in $(seq 100); do
    running || break
    [ "$(sockets)" -ge 2 ] && { echo "exporter started (pid $(pid))"; return 0; }
    sleep 0.1
  done
  echo "exporterctl: the exporter did not come up; its output:" >&2
  cat "$(kd-fixture log exporter)" >&2
  return 1
}

stop() {
  if running; then
    p=$(pid)
    kill "$p"
    for _ in $(seq 50); do kill -0 "$p" 2> /dev/null || break; sleep 0.1; done
    echo "exporter stopped"
  else
    echo "exporter is not running"
  fi
}

case "${1:-}" in
  start)   start ;;
  stop)    stop ;;
  restart) stop; start ;;
  status)
    if running; then
      echo "exporter is running (pid $(pid)); output in $(kd-fixture log exporter)"
    else
      echo "exporter is not running"; exit 3
    fi ;;
  *) echo "usage: sudo $0 start|stop|restart|status" >&2; exit 2 ;;
esac
SH
chmod 755 "$base/exporterctl" "$base/exporter.py"

sha256sum "$base/exporter.py" "$base/exporterctl" > "$base/.state/manifest"
chmod 700 "$base/.state"

# The learner edits the configuration; program and control script stay root's.
chown candidate:candidate "$base" "$base/exporter.conf"

"$base/exporterctl" start > /dev/null
