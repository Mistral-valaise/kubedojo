#!/usr/bin/env bash
# Two Vim sessions that died with unsaved changes, each leaving a real swap
# file: limits.conf (the changes are wanted) and upstream.conf (they are not).
# Vim itself writes the swap files: it edits in ex mode, flushes the buffer to
# the swap file with :preserve and is then killed, which is what a dropped
# connection does to an editor.
set -euo pipefail

base=/opt/course/vim-recover-swap
rm -rf "$base"
mkdir -p "$base/.grade"

cat > "$base/limits.conf" <<'EOF'
# Rate limits of the public API gateway
[default]
requests_per_minute = 600
burst = 50

[login]
requests_per_minute = 30
burst = 5

[search]
requests_per_minute = 300
burst = 40
EOF

cat > "$base/upstream.conf" <<'EOF'
# Upstream servers of the public API gateway
[api]
server = 10.20.7.11:8443
server = 10.20.7.12:8443
server = 10.20.7.13:8443
keepalive = 32
EOF

chown candidate:candidate "$base" "$base/limits.conf" "$base/upstream.conf"

# crash <file> <ex command>...: edit as candidate, never save, die.
crash() {
  local file=$1 args=() c
  shift
  for c in "$@"; do args+=(-c "$c"); done
  (
    cd "$base"
    setpriv --reuid candidate --regid candidate --init-groups \
      env HOME=/home/candidate USER=candidate \
      vim -es -N -u NONE "${args[@]}" -c 'preserve' -c 'call system("kill -9 " . getpid())' "$file" \
      </dev/null >/dev/null 2>&1
  ) 2>/dev/null || true
  [ -s "$base/.$file.swp" ] || { echo "setup: Vim left no swap file for $file" >&2; exit 1; }
  # The swap file records the pid of the Vim that wrote it. That pid is free
  # now, but pids are reused (at the latest after a restart of the container),
  # and Vim would then claim that the session is STILL RUNNING and not offer
  # to delete the swap file. Pid 0 means "unknown": the warning is the same
  # every time. The pid is a 4-byte field at offset 24 of the header block.
  python3 - "$base/.$file.swp" <<'PY'
import sys
with open(sys.argv[1], 'r+b') as swap:
    if swap.read(5) != b'b0VIM':
        sys.exit('setup: unexpected swap file format')
    swap.seek(24)
    swap.write(bytes(4))
PY
}

limits=(
  '/^\[login\]/+1s/30/20/'
  '/^\[search\]/+2s/40/60/'
  'call append(line("$"), ["", "[checkout]", "requests_per_minute = 120", "burst = 10"])'
)

# The wanted result: the same commands, saved, on a copy only root can read.
cp "$base/limits.conf" "$base/.grade/limits.conf"
cp "$base/upstream.conf" "$base/.grade/upstream.conf"
args=()
for c in "${limits[@]}"; do args+=(-c "$c"); done
vim -es -N -u NONE -n "${args[@]}" -c 'wq' "$base/.grade/limits.conf" </dev/null
grep -q '^\[checkout\]$' "$base/.grade/limits.conf" || { echo "setup: the reference copy was not edited" >&2; exit 1; }
chmod 700 "$base/.grade"

crash limits.conf "${limits[@]}"
crash upstream.conf 'g/^server/d' 'call append(line("$"), "server = 127.0.0.1:9999")'
