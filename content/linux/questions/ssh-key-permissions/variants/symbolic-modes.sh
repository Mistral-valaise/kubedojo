#!/usr/bin/env bash
# expect: pass
# The same result with symbolic modes, a read-only key and a config that
# others may read (ssh only minds if they can write it).
set -euo pipefail
d=/opt/course/ssh-key-permissions/deploy
chmod u=rwx,go= "$d/.ssh"
chmod u=r,go= "$d/.ssh/id_ed25519"
chmod go-w "$d/.ssh/config"
setfacl --modify user:sshkeyperm-audit:r-- "$d/release.env"
