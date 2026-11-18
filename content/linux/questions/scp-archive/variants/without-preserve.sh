#!/usr/bin/env bash
# expect: fail preserved
# Everything right except the -p: the copies carry the time of the transfer
# instead of the modification time of their source.
set -euo pipefail
cd /opt/course/scp-archive

opts=(-i keys/archive_ed25519 -o StrictHostKeyChecking=accept-new)
chmod 600 keys/archive_ed25519
scp "${opts[@]}" outbox/release-2.4.1.tar.gz outbox/deploy.sh scp-archive@localhost:incoming/
scp "${opts[@]}" scp-archive@localhost:receipts/release-2.4.0.receipt inbox/
ssh "${opts[@]}" scp-archive@localhost 'cd incoming && sha256sum deploy.sh release-2.4.1.tar.gz' > remote.sha256
