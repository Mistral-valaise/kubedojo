#!/usr/bin/env bash
# expect: pass
# The same result with octal modes, directories and files handled separately.
set -euo pipefail
p=/opt/course/team-shared-dir/project
sudo chown -R :teamdir-dev "$p"
sudo find "$p" -type d -exec chmod 2770 {} +
sudo find "$p" -type f ! -perm /111 -exec chmod 660 {} +
sudo find "$p" -type f -perm /111 -exec chmod 770 {} +
sudo chmod 3770 "$p/inbox"
