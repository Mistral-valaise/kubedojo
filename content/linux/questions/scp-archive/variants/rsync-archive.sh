#!/usr/bin/env bash
# expect: pass
# Another correct way: a read-only key (400), rsync -a over ssh in both
# directions, and the checksums in sha256sum's binary-mode format.
set -euo pipefail
cd /opt/course/scp-archive

chmod 400 keys/archive_ed25519
rsh="ssh -i $PWD/keys/archive_ed25519 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"

rsync -a -e "$rsh" outbox/ scp-archive@localhost:incoming/
rsync -a -e "$rsh" scp-archive@localhost:receipts/release-2.4.0.receipt inbox/
$rsh scp-archive@localhost 'cd ~/incoming; sha256sum -b *' > remote.sha256
