#!/usr/bin/env bash
# expect: pass
# No editor at all: sed changes the one line that starts with the setting.
set -euo pipefail
sed -i 's/^max_connections = .*/max_connections = 250/' /opt/course/vim-first-edit/db.conf
