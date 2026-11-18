#!/usr/bin/env bash
# expect: fail ssh-dir private-key
# "600 for everything that is secret", directory included. Without the
# execute bit nobody can reach the files in it, not even the owner.
set -euo pipefail
cd /opt/course/ssh-key-permissions/deploy
chmod 600 .ssh/id_ed25519 .ssh/config
chmod 600 .ssh
setfacl -m u:sshkeyperm-audit:r release.env
