#!/usr/bin/env bash
# A deploy key whose directory, private key and client configuration are open
# to everybody, and a file with a token that one more account has to read.
set -euo pipefail

base=/opt/course/ssh-key-permissions
auditor=sshkeyperm-audit

# The extra reader: an account without login, home or any group but its own.
if id "$auditor" >/dev/null 2>&1; then
  pkill -KILL -u "$auditor" 2>/dev/null || true
  userdel "$auditor"
fi
getent group "$auditor" >/dev/null && groupdel "$auditor"
useradd --system --no-create-home --user-group --shell /usr/sbin/nologin "$auditor"

rm -rf "$base"
mkdir -p "$base/deploy/.ssh"

ssh-keygen -q -t ed25519 -N '' -C deploy@ssh-key-permissions -f "$base/deploy/.ssh/id_ed25519"
cat > "$base/deploy/.ssh/config" <<'CONF'
Host staging
  HostName staging.internal.example
  User deploy
  IdentityFile /opt/course/ssh-key-permissions/deploy/.ssh/id_ed25519
  IdentitiesOnly yes
CONF
cat > "$base/deploy/release.env" <<'ENV'
# Read by the release job and, since the last audit, by the audit collector.
RELEASE_CHANNEL=stable
REGISTRY_USER=deploy
REGISTRY_TOKEN=kd-7f3a91c2e4b8
ENV

# What the checks compare against. Kept outside the learner's reach.
install -d -m 0700 "$base/.ref"
cut -d' ' -f1,2 "$base/deploy/.ssh/id_ed25519.pub" > "$base/.ref/public-key"
sha256sum < "$base/deploy/release.env" | cut -d' ' -f1 > "$base/.ref/release-env.sha256"

chown -R candidate:candidate "$base"
chown -R root:root "$base/.ref"

# The broken state: somebody "fixed" an access problem with a broad chmod.
chmod 0777 "$base/deploy/.ssh"
chmod 0644 "$base/deploy/.ssh/id_ed25519"
chmod 0644 "$base/deploy/.ssh/id_ed25519.pub"
chmod 0666 "$base/deploy/.ssh/config"
chmod 0600 "$base/deploy/release.env"
