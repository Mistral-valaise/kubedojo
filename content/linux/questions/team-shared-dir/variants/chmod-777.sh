#!/usr/bin/env bash
# expect: fail group outsiders inbox no-exec
# The classic: everybody may do everything. The members can work, and so can
# every other account; nothing inherits the group, and every file is
# executable.
set -euo pipefail
p=/opt/course/team-shared-dir/project
sudo chgrp -R teamdir-dev "$p"
sudo chmod -R 777 "$p"
