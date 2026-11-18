#!/usr/bin/env bash
# A directory tree that stands for a data volume. A container cannot mount a
# small file system to fill up, so the two symptoms come as recorded command
# output (incident.txt), and the tree holds the two culprits for real:
#   - one core dump that uses most of the disk space,
#   - one directory with thousands of empty session files (inodes, no space),
# plus a sparse file that only looks huge.
set -euo pipefail

id=disk-full-or-inodes
base=/opt/course/$id
rm -rf "$base"
mkdir -p "$base/.state"

cat > "$base/incident.txt" <<'TXT'
# Recorded on the production host at the time of the alert. /srv/volume there
# is the tree you find here as /opt/course/disk-full-or-inodes/volume.

$ touch /srv/volume/cache/sessions/probe
touch: cannot touch '/srv/volume/cache/sessions/probe': No space left on device

$ df -h /srv/volume
Filesystem      Size  Used Avail Use% Mounted on
/dev/sdb1        20G   13G  6.2G  68% /srv/volume

$ df -i /srv/volume
Filesystem      Inodes   IUsed IFree IUse% Mounted on
/dev/sdb1      1310720 1310720     0  100% /srv/volume
TXT

python3 - "$base" <<'PY'
import os
import random
import sys
import time

base = sys.argv[1]
vol = base + "/volume"
rnd = random.Random(20391)
now = time.time()
DAY = 86400
keep = []          # what has to survive the clean-up


def put(rel, size=0, age_days=1.0, sparse=False, must_stay=True):
    path = os.path.join(vol, rel)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        if sparse:
            f.truncate(size)
        else:
            f.write(b"\0" * size)
    t = now - age_days * DAY
    os.utime(path, (t, t))
    if must_stay:
        keep.append("f %d ./%s" % (size, rel))


K, M = 1024, 1024 * 1024

# The space: a forgotten core dump. Everything else is small on disk.
put("uploads/tmp/core.20391", 24 * M, 12, must_stay=False)
for n in range(6):
    put("uploads/img/product-%02d.jpg" % n, 150 * K, 30 + n)
put("logs/app.log", 2 * M, 0.1)
put("logs/app.log.1", 1 * M, 1.2)
put("db/wal/000017.log", 512 * K, 0.2)
# Looks like the biggest file by far, but is sparse: 1 GiB long, no blocks used.
put("db/pages.img", 1024 * M, 2, sparse=True)

# The inodes: session files that nobody expires. 2400 are stale, 300 are live.
for n in range(2700):
    name = "cache/sessions/sess_%012x" % rnd.getrandbits(48)
    if n % 9 == 0:
        put(name, 0, rnd.uniform(0.01, 3.0))
    else:
        put(name, 0, rnd.uniform(10, 60), must_stay=False)
# Many files too, but spread over ten directories, and needed.
for n in range(500):
    put("mail/queue/%d/msg_%06d.eml" % (n % 10, n), 300, rnd.uniform(0.01, 20))
put("cache/thumbs/index.db", 64 * K, 3)

for root, dirs, _ in os.walk(vol):
    for d in dirs:
        keep.append("d - ./" + os.path.relpath(os.path.join(root, d), vol))
with open(base + "/.state/keep", "w") as f:
    f.writelines(line + "\n" for line in keep)
PY
LC_ALL=C sort -o "$base/.state/keep" "$base/.state/keep"
chmod 700 "$base/.state"

# The volume belongs to candidate: cleaning it up needs no sudo.
chown -R candidate:candidate "$base/volume"
chown candidate:candidate "$base" "$base/incident.txt"
