#!/usr/bin/env bash
# Two copies of one configuration. web-02 is derived from web-01 with noise
# that does not matter (comments, blank lines, spacing, the two host-specific
# settings) and three changes that do: a value, a missing key, an extra key.
set -euo pipefail

base=/opt/course/config-drift
rm -rf "$base"
mkdir -p "$base/web-01" "$base/web-02"

cat > "$base/web-01/app.conf" <<'CONF'
# shop frontend configuration
# Managed by configuration management, run 2291 (web-01)

# --- identity ---
hostname = web-01
listen_address = 10.20.0.11
listen_port = 8443
environment = production

# --- workers ---
worker_processes = 8
worker_connections = 2048
worker_shutdown_seconds = 25
keepalive_seconds = 65
keepalive_requests = 1000

# --- tls ---
tls_certificate = /etc/shop/tls/fullchain.pem
tls_private_key = /etc/shop/tls/privkey.pem
tls_min_version = 1.2
tls_session_cache_mb = 20
tls_session_tickets = off
hsts_max_age = 31536000

# --- upstream ---
upstream_url = http://api.shop.internal:9000
upstream_connect_timeout_seconds = 3
upstream_read_timeout_seconds = 30
upstream_retries = 2
upstream_keepalive = 32

# --- limits ---
max_body_size_mb = 10
max_header_size_kb = 16
rate_limit_per_minute = 120
rate_limit_burst = 40
rate_limit_status = 429

# --- cache ---
cache_enabled = on
cache_path = /var/cache/shop
cache_max_size_mb = 2048
cache_ttl_seconds = 300
cache_stale_seconds = 60

# --- logging ---
log_level = warn
log_format = json
log_access = /var/log/shop/access.log
log_error = /var/log/shop/error.log
log_sample_rate = 1.0

# --- misc ---
maintenance_page = /srv/shop/maintenance.html
motd = Welcome to the shop
feature_new_checkout = off
feature_recommendations = on
CONF

sed -e 's/run 2291 (web-01)/run 2274 (web-02)/' \
    -e 's/^hostname = web-01/hostname = web-02/' \
    -e 's/^listen_address = 10.20.0.11/listen_address = 10.20.0.12/' \
    -e 's/^\(worker_[a-z_]*\) = /\1=/' \
    -e 's/^\(cache_[a-z_]*\) = /\1\t=\t/' \
    -e 's/^\(upstream_[a-z_]*\) = /\1  =  /' \
    -e 's/^\(log_[a-z]*\) = \(.*\)$/\1 = \2   /' \
    -e 's/^# --- \(.*\) ---$/## \1/' \
    -e 's/^tls_min_version = 1.2$/tls_min_version = 1.0/' \
    -e '/^rate_limit_per_minute/d' \
    -e '/^log_level/a debug_endpoints = on' \
    -e '/^keepalive_requests/a\\n# raised after the load test in January' \
    -e '/^## misc/i # TODO: move the cache to the new volume\n' \
    "$base/web-01/app.conf" > "$base/web-02/app.conf"

sha256sum "$base/web-01/app.conf" | cut -d' ' -f1 > "$base/.manifest"

# The learner repairs web-02 and writes drift.txt; the reference stays root's.
chown candidate:candidate "$base" "$base/web-02" "$base/web-02/app.conf"
