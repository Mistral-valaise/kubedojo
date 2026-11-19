#!/usr/bin/env bash
# A load balancer configuration with three column jobs: comment out six
# lines, uncomment six others, and put the same prefix in front of six more.
set -euo pipefail

base=/opt/course/vim-block-edit
rm -rf "$base"
mkdir -p "$base/.grade"

cat > "$base/haproxy.cfg" <<'EOF'
# Load balancer of the shop. Blue is the live release, green the new one.
frontend admin
    bind *:8404
    # networks that may reach the admin interface
    10.20.1.0/24
    10.20.2.0/24
    10.20.8.0/24
    10.30.0.0/16
    172.16.9.0/24
    192.168.40.0/24
    http-request deny if !admin_net

frontend shop
    bind *:443
    default_backend shop

backend shop
    balance roundrobin
    option httpchk GET /healthz
    # blue pool
    server blue-1 10.20.5.11:8080 check
    server blue-2 10.20.5.12:8080 check
    server blue-3 10.20.5.13:8080 check
    server blue-4 10.20.5.14:8080 check
    server blue-5 10.20.5.15:8080 check
    server blue-6 10.20.5.16:8080 check
    # green pool
    # server green-1 10.20.6.11:8080 check
    # server green-2 10.20.6.12:8080 check
    # server green-3 10.20.6.13:8080 check
    # server green-4 10.20.6.14:8080 check
    # server green-5 10.20.6.15:8080 check
    # server green-6 10.20.6.16:8080 check
EOF

cp "$base/haproxy.cfg" "$base/.grade/original"
chmod 700 "$base/.grade"
chown candidate:candidate "$base" "$base/haproxy.cfg"
