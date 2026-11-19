#!/usr/bin/env bash
# A report job that runs as its own account and fails with "Permission
# denied". Three things are wrong on the way to its configuration; the account
# and its group membership are fine.
set -euo pipefail

base=/opt/course/unreadable-config
svc=unreadable-config-svc
ops=unreadable-config-ops

if id "$svc" >/dev/null 2>&1; then
  pkill -KILL -u "$svc" 2>/dev/null || true
  userdel "$svc"
fi
for g in "$svc" "$ops"; do
  if getent group "$g" >/dev/null; then groupdel "$g"; fi
done
groupadd --system "$ops"
useradd --system --no-create-home --user-group --groups "$ops" --shell /usr/sbin/nologin "$svc"

rm -rf "$base"
mkdir -p "$base/app/conf"

cat > "$base/app/run-report.sh" <<'SCRIPT'
#!/usr/bin/env bash
# Prints the header of the nightly report. Runs as unreadable-config-svc.
set -euo pipefail
conf=/opt/course/unreadable-config/app/conf/report.conf
# shellcheck disable=SC1090
. "$conf"
echo "report ok: region=$REGION recipients=$RECIPIENTS"
SCRIPT
cat > "$base/app/conf/report.conf" <<'CONF'
# Settings of the nightly report. Contains a credential: not for everybody.
REGION=eu-central
RECIPIENTS=3
SMTP_PASSWORD=kd-2b7e151628ae
CONF

install -d -m 0700 "$base/.ref"
sha256sum < "$base/app/conf/report.conf" | cut -d' ' -f1 > "$base/.ref/report-conf.sha256"
sha256sum < "$base/app/run-report.sh" | cut -d' ' -f1 > "$base/.ref/run-report.sha256"

chown candidate:candidate "$base"

# The application belongs to root; the group of operators may use it.
chown -R root:"$ops" "$base/app"
chmod 0750 "$base/app"                    # fine
chmod 0640 "$base/app/run-report.sh"      # fault: not executable
chown root:root "$base/app/conf"
chmod 0700 "$base/app/conf"               # fault: wrong group, nobody but root gets in
chmod 0600 "$base/app/conf/report.conf"   # fault: right group, but the group may not read
