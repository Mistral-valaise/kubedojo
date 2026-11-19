#!/usr/bin/env bash
# Five "services" of a status page. One is healthy; the other four fail at
# three different layers: a name that does not resolve, a port nobody listens
# on, and two servers that answer with an error status.
set -euo pipefail

id=net-layer-diagnosis
base=/opt/course/$id
kd-fixture reset
# Also what a learner started by hand on these ports (it carries no KD_QUESTION).
for port in 23011 23012 23013 23014 23015; do
  fuser -s -k -KILL "$port/tcp" 2>/dev/null || true
done

rm -rf "$base"
mkdir -p "$base/.fixture"

cat > "$base/.fixture/server.py" <<'PY'
"""Status page fixture: one process, three ports, fixed answers."""
import errno
import http.server
import threading
import time

# port -> {path: (status, body)}; every other path is a 404.
ROUTES = {
    23011: {"/health": (200, "ok\n")},
    23014: {"/health": (503, "database unavailable\n")},
    23015: {"/healthz": (200, "ok\n")},
}


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        routes = ROUTES[self.server.server_address[1]]
        status, body = routes.get(self.path, (404, "not found\n"))
        data = body.encode()
        self.send_response(status)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(data)

    do_HEAD = do_GET

    def log_message(self, *args):
        pass


def listen(port):
    # A port freed a moment ago by a reset may still be busy for an instant.
    for _ in range(50):
        try:
            return http.server.ThreadingHTTPServer(("0.0.0.0", port), Handler)
        except OSError as e:
            if e.errno != errno.EADDRINUSE:
                raise
            time.sleep(0.1)
    raise SystemExit(f"port {port} is in use")


servers = [listen(p) for p in ROUTES]
for s in servers[1:]:
    threading.Thread(target=s.serve_forever, daemon=True).start()
servers[0].serve_forever()
PY
chmod 700 "$base/.fixture"

kd-fixture start web -- python3 "$base/.fixture/server.py"
# The setup is done when the ports answer, not when the process exists.
for port in 23011 23014 23015; do
  for _ in $(seq 100); do
    ss -Hltn "sport = :$port" | grep -q . && break
    sleep 0.1
  done
  ss -Hltn "sport = :$port" | grep -q . || { echo "port $port did not come up" >&2; exit 1; }
done

host=$(hostname)
cat > "$base/endpoints.txt" <<LIST
billing    http://$host:23011/health
inventory  http://inventory.$id.invalid:23012/health
search     http://$host:23013/health
checkout   http://$host:23014/health
profile    http://$host:23015/health
LIST

chown candidate:candidate "$base" "$base/endpoints.txt"
