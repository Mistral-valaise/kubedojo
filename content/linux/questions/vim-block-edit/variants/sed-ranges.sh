#!/usr/bin/env bash
# expect: pass
# sed with patterns instead of a block selection. The comment sign goes to
# column 1 and without a space: another accepted way to comment a line out.
set -euo pipefail
sed -i -E \
  -e '/server blue-[1-6] /s/^/#/' \
  -e '/server green-[1-6] /s/^( *)# ?/\1/' \
  -e '/^ *[0-9.]+\/[0-9]+$/s/^( *)/\1acl admin_net src /' \
  /opt/course/vim-block-edit/haproxy.cfg
