#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/scp-archive

key=keys/archive_ed25519
remote=scp-archive@localhost
# accept-new answers the "are you sure" question of a first connection.
opts=(-i "$key" -o StrictHostKeyChecking=accept-new)

chmod 600 "$key"
scp -p "${opts[@]}" outbox/release-2.4.1.tar.gz outbox/deploy.sh "$remote:incoming/"
scp -p "${opts[@]}" "$remote:receipts/release-2.4.0.receipt" inbox/
ssh "${opts[@]}" "$remote" 'cd incoming && sha256sum deploy.sh release-2.4.1.tar.gz' > remote.sha256

# The proof: the remote checksums fit the local files.
(cd outbox && sha256sum -c ../remote.sha256)
