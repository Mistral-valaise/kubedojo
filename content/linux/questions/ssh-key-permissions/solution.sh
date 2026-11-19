#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/ssh-key-permissions/deploy

chmod 700 .ssh
chmod 600 .ssh/id_ed25519 .ssh/config

# One more reader, without opening the file to a group or to everybody.
setfacl -m u:sshkeyperm-audit:r release.env
