#!/usr/bin/env bash
# One application log of 420 lines from six services. It is generated with
# plain integer arithmetic, so it is the same on every run. Traps: INFO lines
# that mention the word ERROR (all of service search's "errors" are of that
# kind), and the service payments-worker, whose name contains "payments" and
# whose ERROR lines start before and end after those of payments.
set -euo pipefail

base=/opt/course/error-log-triage
rm -rf "$base"
mkdir -p "$base"

python3 - > "$base/app.log" <<'PY'
services = ["auth", "gateway", "payments", "payments-worker", "search", "notifier"]
x = 20240514
def rnd(n):
    global x
    x = (x * 1103515245 + 12345) % 2147483648
    return (x >> 8) % n

t = 9 * 3600
for i in range(1, 421):
    t += 1 + rnd(40)
    svc = services[rnd(len(services))]
    r = rnd(100)
    level = "DEBUG" if r < 14 else "INFO" if r < 70 else "WARN" if r < 88 else "ERROR"
    n = 20 + rnd(900)
    # search and notifier never fail; payments fails only in the middle of the log.
    if level == "ERROR" and (svc in ("search", "notifier") or (svc == "payments" and not 60 <= i <= 360)):
        level = "WARN"
    if i in (20, 400):
        svc, level = "payments", "INFO"
    if i in (30, 390):
        svc, level = "payments-worker", "ERROR"
    if level == "DEBUG":
        msg = "payload size %d bytes" % n
    elif level == "INFO":
        if svc == "search" and n % 3 == 0:
            msg = "index healthy, no ERROR in the last %d queries" % n
        elif i in (20, 400):
            msg = "ERROR counter reset to 0"
        else:
            msg = ("request served in %d ms" % n, "cache refreshed", "health check ok")[n % 3]
    elif level == "WARN":
        msg = ("retrying upstream call (%d/3)" % (1 + n % 3), "slow response: %d ms" % (n * 3))[n % 2]
    else:
        msg = ("upstream timeout after %d ms" % (n * 10), "connection refused by db-%d" % (n % 4))[n % 2]
    print("2024-05-14 %02d:%02d:%02d [%s] %s %s" % (t // 3600, t % 3600 // 60, t % 60, svc, level, msg))
PY

sha256sum "$base/app.log" | cut -d' ' -f1 > "$base/.manifest"
chown candidate:candidate "$base"
