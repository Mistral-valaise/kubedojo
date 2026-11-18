#!/usr/bin/env bash
# expect: fail no-exec
# One recursive chmod for directories and files alike: the access is right,
# but every text file is now executable and carries a setgid bit.
set -euo pipefail
p=/opt/course/team-shared-dir/project
sudo chgrp -R teamdir-dev "$p"
sudo chmod -R 2770 "$p"
sudo chmod +t "$p/inbox"
