#!/usr/bin/env bash
set -euo pipefail
p=/opt/course/team-shared-dir/project

# Everything belongs to the team's group.
sudo chgrp -R teamdir-dev "$p"
# Group: read and write; X = execute only for directories and for files that
# are executable already. Others: nothing.
sudo chmod -R u+rwX,g+rwX,o-rwx "$p"
# setgid on every directory: what is created inside inherits the group.
sudo find "$p" -type d -exec chmod g+s {} +
# Sticky bit: in inbox only the owner of a file may delete or rename it.
sudo chmod +t "$p/inbox"
