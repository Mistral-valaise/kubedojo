#!/usr/bin/env bash
# Runs on every start of the lab container, also after a restart: installs the
# session's ssh key ($KD_PUBKEY) for root and candidate and starts sshd.
#
# sshd runs as a daemon next to the main process on purpose: an exercise may
# stop or restart it (`service ssh restart`) without ending the container.
set -euo pipefail

if [ -n "${KD_PUBKEY:-}" ]; then
  for home in /root /home/candidate; do
    mkdir -p "$home/.ssh"
    echo "$KD_PUBKEY" > "$home/.ssh/authorized_keys"
    chmod 700 "$home/.ssh"; chmod 600 "$home/.ssh/authorized_keys"
  done
  chown -R candidate:candidate /home/candidate/.ssh
fi

# Host keys belong to the container, not to the image everybody pulls.
ssh-keygen -A >/dev/null
# After a restart the pids in old pidfiles belong to other processes.
rm -rf /run/kd-fixture /run/sshd.pid
mkdir -p /run/sshd
/usr/sbin/sshd

exec sleep infinity
