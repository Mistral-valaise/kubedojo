#!/usr/bin/env bash
# Three services, three log formats, one request id that ties them together.
# gateway -> orders -> payments; the logs are generated with a fixed seed, so
# every run produces the same lines.
set -euo pipefail

id=trace-request-id
base=/opt/course/$id
rm -rf "$base"
mkdir -p "$base/logs" "$base/.state"

python3 - "$base" <<'PY'
import datetime
import json
import random
import sys

base = sys.argv[1]
rnd = random.Random(7731)
TARGET, RETRY, ORDER = "7f3a9c", "b81d02", "ORD-7731"
start = datetime.datetime(2026, 11, 18, 10, 0, 0, 0)

gateway, orders, payments, trace = [], [], [], []
used = {TARGET, RETRY}


def rid():
    while True:
        r = "%06x" % rnd.randrange(16 ** 6)
        if r not in used and TARGET not in r:
            used.add(r)
            return r


def txn():
    while True:
        t = "%08x" % rnd.randrange(16 ** 8)
        if TARGET not in t:
            return t


def gw(t, r, rest):
    line = "%sZ gateway req=%s %s" % (t.isoformat(timespec="milliseconds"), r, rest)
    gateway.append((t, line))
    return line


def od(t, r, level, msg, order):
    line = json.dumps({"ts": t.isoformat(timespec="milliseconds") + "Z", "level": level, "svc": "orders",
                       "request_id": r, "msg": msg, "order": order}, separators=(",", ":"))
    orders.append((t, line))
    return line


def pay(t, r, level, rest):
    line = "[%s] payments %-5s rid=%s %s" % (t.isoformat(sep=" ", timespec="milliseconds"), level, r, rest)
    payments.append((t, line))
    return line


def ms(n):
    return datetime.timedelta(milliseconds=n)


def create(t, r, order, outcome, forced_txn=None):
    """One POST /orders; returns the lines it wrote."""
    out = [gw(t, r, "POST /orders order=%s client=10.20.3.%d" % (order, rnd.randrange(10, 60)))]
    t += ms(rnd.randrange(15, 40))
    out.append(od(t, r, "info", "create order", order))
    if outcome == "invalid":
        t += ms(4)
        out.append(od(t, r, "warn", "validation failed: quantity must be positive", order))
        out.append(gw(t + ms(6), r, "status=422 upstream=orders dur=%dms" % rnd.randrange(30, 60)))
        return out
    if outcome == "db":
        t += ms(210)
        out.append(od(t, r, "error", "insert failed: duplicate key orders_pkey", order))
        out.append(gw(t + ms(7), r, "status=500 upstream=orders dur=%dms" % rnd.randrange(230, 280)))
        return out
    t += ms(rnd.randrange(20, 50))
    out.append(od(t, r, "info", "calling payments", order))
    t += ms(rnd.randrange(10, 30))
    amount = "%d.%02d" % (rnd.randrange(5, 240), rnd.choice([0, 50, 90, 95, 99]))
    out.append(pay(t, r, "INFO", "charge start order=%s amount=%s" % (order, amount)))
    if outcome == "timeout":
        out.append(pay(t + ms(2001), r, "WARN", "card-processor slow, no answer after 2000ms"))
        t += ms(3004)
        out.append(pay(t, r, "ERROR", "card-processor timeout after 3000ms, giving up"))
        t += ms(9)
        out.append(od(t, r, "error", "payments returned 504", order))
        out.append(gw(t + ms(8), r, "status=502 upstream=orders dur=%dms" % rnd.randrange(3100, 3200)))
        return out
    t += ms(rnd.randrange(120, 420))
    if outcome == "declined":
        out.append(pay(t, r, "WARN", "card declined code=insufficient_funds"))
        t += ms(6)
        out.append(od(t, r, "warn", "payments returned 402", order))
        out.append(gw(t + ms(5), r, "status=402 upstream=orders dur=%dms" % rnd.randrange(200, 500)))
        return out
    out.append(pay(t, r, "INFO", "charge ok txn=%s" % (forced_txn or txn())))
    t += ms(8)
    out.append(od(t, r, "info", "order confirmed", order))
    out.append(gw(t + ms(6), r, "status=201 upstream=orders dur=%dms" % rnd.randrange(200, 500)))
    return out


def fetch(t, r, order):
    gw(t, r, "GET /orders/%s client=10.20.3.%d" % (order, rnd.randrange(10, 60)))
    od(t + ms(12), r, "info", "fetch order", order)
    gw(t + ms(21), r, "status=200 upstream=orders dur=%dms" % rnd.randrange(15, 40))


# Background traffic: one request about every 20 seconds for half an hour.
special = {12: "invalid", 23: "declined", 31: "db", 44: "timeout", 58: "declined", 71: "invalid"}
t = start
for n in range(90):
    t += datetime.timedelta(seconds=rnd.randrange(14, 26), milliseconds=rnd.randrange(1000))
    order = "ORD-%d" % (7600 + n)
    if n % 4 == 3:
        fetch(t, rid(), "ORD-%d" % rnd.randrange(7000, 7600))
    else:
        # One unrelated charge whose transaction id happens to contain the target id.
        create(t, rid(), order, special.get(n, "ok"), forced_txn="a0" + TARGET if n == 50 else None)

# The complaint: ORD-7731 fails at 10:15:01 and goes through on the second attempt.
trace = create(datetime.datetime(2026, 11, 18, 10, 15, 1, 204000), TARGET, ORDER, "timeout")
create(datetime.datetime(2026, 11, 18, 10, 16, 40, 87000), RETRY, ORDER, "ok")
fetch(datetime.datetime(2026, 11, 18, 10, 16, 52, 310000), rid(), ORDER)

for name, lines in (("gateway", gateway), ("orders", orders), ("payments", payments)):
    lines.sort(key=lambda x: x[0])
    with open("%s/logs/%s.log" % (base, name), "w") as f:
        f.writelines(line + "\n" for _, line in lines)
with open(base + "/.state/trace.expected", "w") as f:
    f.writelines(line + "\n" for line in trace)
PY

# Logs belong to root and are read-only for everybody else, as on a real host.
chmod 644 "$base"/logs/*.log
(cd "$base/logs" && sha256sum gateway.log orders.log payments.log) > "$base/.state/manifest"
chmod 700 "$base/.state"
chown candidate:candidate "$base"
