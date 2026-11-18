#!/usr/bin/env bash
# A second account on the same host plays the remote "archive" machine: it is
# reached over real ssh with a key pair, which is all scp and rsync need.
# The private key is world-readable, so ssh refuses to use it.
set -euo pipefail

id=scp-archive
user=scp-archive
base=/opt/course/$id
home=/home/$user

kd-fixture reset
if id "$user" > /dev/null 2>&1; then
  # Open ssh sessions of the account would keep userdel from removing it.
  pkill -KILL -u "$user" 2> /dev/null || true
  for _ in $(seq 50); do
    pgrep -u "$user" > /dev/null || break
    sleep 0.1
  done
  userdel "$user"
fi
rm -rf "$base" "$home"
useradd -m -s /bin/bash -c "release archive ($id)" "$user"

mkdir -p "$base/keys" "$base/outbox" "$base/inbox" "$base/.state"

# The key pair of the release job.
ssh-keygen -q -t ed25519 -N '' -C "release-job@$id" -f "$base/keys/archive_ed25519"
install -d -m 700 -o "$user" -g "$user" "$home/.ssh"
install -m 600 -o "$user" -g "$user" "$base/keys/archive_ed25519.pub" "$home/.ssh/authorized_keys"
chmod 644 "$base/keys/archive_ed25519"          # the fault: "UNPROTECTED PRIVATE KEY FILE"

# What has to be shipped.
build=$(mktemp -d)
mkdir -p "$build/release-2.4.1/bin" "$build/release-2.4.1/conf"
printf '#!/bin/sh\nexec /opt/shop/bin/shopd --config /opt/shop/conf/shop.conf\n' > "$build/release-2.4.1/bin/start"
printf 'version = 2.4.1\nworkers = 8\n' > "$build/release-2.4.1/conf/shop.conf"
head -c 150000 /dev/urandom > "$build/release-2.4.1/bin/shopd"
tar -czf "$base/outbox/release-2.4.1.tar.gz" -C "$build" release-2.4.1
rm -rf "$build"
cat > "$base/outbox/deploy.sh" <<'SH'
#!/bin/sh
# Unpacks a release below /opt/shop and switches the "current" link to it.
set -eu
version=${1:?usage: deploy.sh <version>}
tar -xzf "release-$version.tar.gz" -C /opt/shop/releases
ln -sfn "/opt/shop/releases/release-$version" /opt/shop/current
SH
chmod 640 "$base/outbox/release-2.4.1.tar.gz"
chmod 750 "$base/outbox/deploy.sh"
touch -d '2026-10-28 16:40:00' "$base/outbox/release-2.4.1.tar.gz"
touch -d '2026-10-28 16:42:10' "$base/outbox/deploy.sh"

# The archive account: an empty incoming directory and the receipt of the last release.
install -d -m 750 -o "$user" -g "$user" "$home/incoming" "$home/receipts"
cat > "$home/receipts/release-2.4.0.receipt" <<'TXT'
release:  release-2.4.0.tar.gz
received: 2026-10-14T08:04:57Z
sha256:   5f0c9a4e7d1b83c2a6e94f1d0b7c35a8e2d6f4091b3a7c5e8d2f60a1b4c7e9d3
status:   archived
TXT
chown "$user:$user" "$home/receipts/release-2.4.0.receipt"
chmod 600 "$home/receipts/release-2.4.0.receipt"
touch -d '2026-10-14 08:05:00' "$home/receipts/release-2.4.0.receipt"

chown -R candidate:candidate "$base"

# What the guard compares against. Root's, so that it is not edited by accident.
{
  sha256sum "$base/keys/archive_ed25519" "$home/.ssh/authorized_keys" "$base/outbox/release-2.4.1.tar.gz" "$base/outbox/deploy.sh"
  stat -c '%a %Y %n' "$base/outbox/release-2.4.1.tar.gz" "$base/outbox/deploy.sh"
} > "$base/.state/manifest"
chown -R root:root "$base/.state"
chmod 700 "$base/.state"
