#!/usr/bin/env bash
# A passwd-like export with 38 accounts. The counts per shell are all
# different, so the ranking has one right order; the UIDs have three to five
# digits, so a text sort ranks them wrongly; and the two header lines have no
# colon, which makes `cut` print them whole.
set -euo pipefail

base=/opt/course/rank-login-shells
rm -rf "$base"
mkdir -p "$base"

{
  echo '# account export of host build-07'
  echo '# format as in /etc/passwd'
  # name uid shell (GECOS and home are derived)
  while read -r name uid shell; do
    printf '%s:x:%s:%s:%s account:/home/%s:%s\n' "$name" "$uid" "$uid" "$name" "$name" "$shell"
  done <<'ACCOUNTS'
alba 1001 /bin/bash
backup 34 /usr/sbin/nologin
benno 1002 /bin/bash
carla 1017 /bin/zsh
ci-runner 998 /bin/sh
daemon 1 /usr/sbin/nologin
dario 20450 /bin/bash
deploy 995 /bin/bash
elif 1033 /bin/bash
fenna 9100 /bin/zsh
ftp 107 /bin/false
games 5 /usr/sbin/nologin
goran 1040 /bin/bash
hana 31007 /bin/bash
ines 1058 /usr/bin/fish
jobs 997 /bin/sh
jonas 1061 /bin/bash
kira 8750 /bin/sh
lp 7 /usr/sbin/nologin
luca 1079 /bin/bash
mail 8 /usr/sbin/nologin
metrics 996 /bin/sh
mira 12003 /bin/zsh
mysql 110 /bin/false
news 9 /usr/sbin/nologin
nobody 65534 /usr/sbin/nologin
noor 1090 /bin/bash
olek 1101 /bin/bash
oracle-old 54321 /usr/sbin/nologin
pia 1112 /bin/bash
proxy 13 /usr/sbin/nologin
rsyncd 994 /bin/sh
sven 999 /bin/zsh
sys 3 /usr/sbin/nologin
tara 1125 /bin/bash
uucp 10 /usr/sbin/nologin
vito 2300 /bin/bash
www-data 33 /bin/sh
ACCOUNTS
} > "$base/accounts.txt"

sha256sum "$base/accounts.txt" | cut -d' ' -f1 > "$base/.manifest"
chown candidate:candidate "$base"
