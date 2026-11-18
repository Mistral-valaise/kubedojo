#!/usr/bin/env bash
# A small database configuration. One value has to change; a comment and two
# other settings contain the same number, so "replace 100 everywhere" is wrong.
set -euo pipefail

base=/opt/course/vim-first-edit
rm -rf "$base"
mkdir -p "$base/.grade"

cat > "$base/db.conf" <<'EOF'
# Configuration of the orders database
# Defaults: max_connections = 100, port = 5432

# Connections
listen_addresses = '*'
port = 5432
max_connections = 100
superuser_reserved_connections = 3

# Memory
shared_buffers = 100MB
work_mem = 4MB
maintenance_work_mem = 64MB

# Logging
log_destination = 'stderr'
log_min_duration_statement = 100
log_line_prefix = '%m [%p] '
EOF

# What the guard compares against. Only root can read it.
cp "$base/db.conf" "$base/.grade/original"
chmod 700 "$base/.grade"
chown candidate:candidate "$base" "$base/db.conf"
