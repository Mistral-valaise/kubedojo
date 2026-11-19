#!/usr/bin/env bash
# The directory of an application after an incident. The vendor asks for a
# support bundle: configuration, logs and the start script, without the
# secrets that live next to them (a private key, an env file, and a password
# inside the configuration file itself).
set -euo pipefail

id=support-bundle
base=/opt/course/$id
app=$base/app
rm -rf "$base"
mkdir -p "$app/bin" "$app/etc/tls" "$app/log/debug" "$app/var/cache" "$base/.state"

password='Xk7-pine-Quartz-41'
apikey='ak_live_4e9c1b7d20aa53f1'
keyline='MIIEvQIBADANBgkqhkiG9w0BAQEFAASCkdSupportBundle7Jq2'

cat > "$app/bin/run.sh" <<'SH'
#!/bin/sh
# Starts the shop with the settings from etc/.
set -eu
cd "$(dirname "$0")/.."
. etc/db.env
exec ./bin/shopd --config etc/app.conf --limits etc/limits.conf
SH

cat > "$app/etc/app.conf" <<CONF
# shop configuration
name = shop
listen = 0.0.0.0:8443
log_level = info

db_host = db.internal
db_name = shop
db_user = shop
db_password = $password

tls_cert = etc/tls/server.crt
tls_key = etc/tls/server.key
CONF
printf 'max_connections = 200\nmax_request_bytes = 1048576\nrequest_timeout = 30s\n' > "$app/etc/limits.conf"
printf 'new_checkout = on\nlegacy_export = off\n' > "$app/etc/.features"
printf 'DB_PASSWORD=%s\nAPI_KEY=%s\n' "$password" "$apikey" > "$app/etc/db.env"
printf '%s\n' '-----BEGIN CERTIFICATE-----' 'MIIDdzCCAl+gAwIBAgIUJq2kdSupportBundlePublicPartOnly0wDQYJKoZIhv' \
  '-----END CERTIFICATE-----' > "$app/etc/tls/server.crt"
printf '%s\n' '-----BEGIN PRIVATE KEY-----' "$keyline" '-----END PRIVATE KEY-----' > "$app/etc/tls/server.key"

{
  for n in $(seq 1 40); do
    printf '2026-11-18T09:%02d:%02dZ INFO  request %04d served in %dms\n' "$((n / 2))" "$((n * 7 % 60))" "$n" "$((n * 13 % 90 + 5))"
  done
  echo '2026-11-18T09:21:07Z ERROR database: too many connections (200/200)'
  echo '2026-11-18T09:21:07Z ERROR request 0041 failed: no database connection'
} > "$app/log/app.log"
for n in $(seq 1 25); do
  printf '2026-11-17T22:%02d:00Z INFO  nightly export step %02d done\n' "$n" "$n"
done > "$app/log/app.log.1"
head -c 2M /dev/urandom > "$app/log/debug/trace.bin"
for n in 1 2 3; do head -c 100K /dev/urandom > "$app/var/cache/blob-$n"; done
printf '{"last_export": "2026-11-17T22:25:00Z", "open_orders": 17}\n' > "$app/var/state.json"

chmod 750 "$app/bin/run.sh"
chmod 640 "$app/etc/app.conf"
chmod 644 "$app/etc/limits.conf" "$app/etc/.features" "$app/etc/tls/server.crt" "$app/var/state.json"
chmod 600 "$app/etc/db.env" "$app/etc/tls/server.key"
chmod 660 "$app/log/app.log" "$app/log/app.log.1"
find "$app" -exec touch -d '2026-11-18 09:21:07' {} +
touch -d '2026-11-17 22:25:00' "$app/log/app.log.1"
touch -d '2026-10-02 11:00:00' "$app/bin/run.sh" "$app/etc/limits.conf" "$app/etc/tls/server.crt" "$app/etc/tls/server.key"

# For the checks: the strings that must not leave the host, and the tree as it was.
printf '%s\n' "$password" "$apikey" "$keyline" > "$base/.state/secrets"
(cd "$base" && find app \( -type f -printf 'f %m %s %p ' -exec sha256sum {} \; \) -o -printf '%y %m - %p\n' \
  | awk '{print $1, $2, $3, $4, $5}' | LC_ALL=C sort) > "$base/.state/originals"
chmod 700 "$base/.state"

# The application directory is candidate's: reading it needs no sudo.
chown -R candidate:candidate "$app"
chown candidate:candidate "$base"
