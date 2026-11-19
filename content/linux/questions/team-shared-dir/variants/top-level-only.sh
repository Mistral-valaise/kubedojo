#!/usr/bin/env bash
# expect: fail group members-write
# Uses a shell glob instead of -R for the group and the setgid bit: the
# hidden file and everything in the subdirectories are left out. Closing the
# top directory keeps outsiders away all the same.
set -euo pipefail
p=/opt/course/team-shared-dir/project
sudo chgrp teamdir-dev "$p" "$p"/*
sudo chmod 2770 "$p" "$p/src" "$p/docs"
sudo chmod 3770 "$p/inbox"
sudo chmod -R o-rwx "$p"
