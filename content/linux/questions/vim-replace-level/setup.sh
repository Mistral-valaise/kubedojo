#!/usr/bin/env bash
# A logging configuration in which the value "warn" has to become "warning".
# The same four letters appear in setting names, in comments, inside another
# value and in values that are already correct.
set -euo pipefail

base=/opt/course/vim-replace-level
rm -rf "$base"
mkdir -p "$base/.grade"

cat > "$base/logging.conf" <<'EOF'
# Logging configuration of the checkout service
# Levels: debug, info, warning, error. The old name "warn" is no longer accepted.
# Loggers that warn too often are rate limited, see warn_rate_limit.

[handlers]
console.level = warn
file.level = info
syslog.level = warn
audit.level = error

[loggers]
root.level = warn
http.level = info
http.access.level = warn
db.level = warning
db.slow_query.level = warn
cache.level = debug
payment.level = warning
payment.prewarn.level = info
queue.level = warn
scheduler.level = warn

[options]
warnings_as_errors = false
warn_rate_limit = 20
prewarn_seconds = 30
on_warn = notify     # warn the on-call channel
message_prefix = "warn: "
EOF

cp "$base/logging.conf" "$base/.grade/original"
chmod 700 "$base/.grade"
chown candidate:candidate "$base" "$base/logging.conf"
