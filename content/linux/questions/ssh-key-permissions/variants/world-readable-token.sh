#!/usr/bin/env bash
# expect: fail private
# The modes of the key are right, but the token is opened to everybody
# instead of to the one account that needs it. The auditor can read it, and
# so can every other user on the host.
set -euo pipefail
cd /opt/course/ssh-key-permissions/deploy
chmod 700 .ssh
chmod 600 .ssh/id_ed25519 .ssh/config
chmod 644 release.env
