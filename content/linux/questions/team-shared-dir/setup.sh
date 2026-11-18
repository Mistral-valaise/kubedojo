#!/usr/bin/env bash
# A project directory that grew without a plan: files of root, of candidate
# and of one developer, with whatever group and mode they were created with.
set -euo pipefail

base=/opt/course/team-shared-dir
group=teamdir-dev
members="teamdir-ana teamdir-ben"
outsider=teamdir-eve

for u in $members $outsider; do
  if id "$u" >/dev/null 2>&1; then
    pkill -KILL -u "$u" 2>/dev/null || true
    userdel "$u"
  fi
  if getent group "$u" >/dev/null; then groupdel "$u"; fi
done
if getent group "$group" >/dev/null; then groupdel "$group"; fi

groupadd "$group"
for u in $members; do
  useradd --no-create-home --user-group --groups "$group" --shell /bin/bash "$u"
done
useradd --no-create-home --user-group --shell /bin/bash "$outsider"

rm -rf "$base"
p=$base/project
mkdir -p "$p/src/lib" "$p/docs" "$p/inbox"

printf '%s\n' '# Tariff calculator' '' 'Shared workspace of the developers in group teamdir-dev.' > "$p/README.md"
printf '%s\n' 'root = true' '[*]' 'indent_style = space' > "$p/.editorconfig"
printf '%s\n' 'def tariff(kwh):' '    return round(kwh * 0.31, 2)' > "$p/src/app.py"
printf '%s\n' 'RATE = 0.31' > "$p/src/lib/rates.py"
printf '%s\n' '#!/usr/bin/env bash' 'python3 -m py_compile src/app.py && echo build ok' > "$p/src/build.sh"
printf '%s\n' 'Open points' '- rounding of the night tariff' > "$p/docs/notes.txt"
printf '%s\n' 'Please review: src/lib/rates.py' > "$p/inbox/from-ana.txt"
printf '%s\n' 'Draft of the release note' > "$p/inbox/from-ben.txt"

# Owners, groups and modes as they happened to be.
chown -R root:root "$p"
chmod 0755 "$p" "$p/src" "$p/src/lib" "$p/docs" "$p/inbox"
chmod 0644 "$p/README.md" "$p/.editorconfig" "$p/src/lib/rates.py"
chmod 0755 "$p/src/build.sh"
chown candidate:candidate "$p/src/app.py";      chmod 0600 "$p/src/app.py"
chown teamdir-ana:teamdir-ana "$p/docs" "$p/docs/notes.txt"; chmod 0640 "$p/docs/notes.txt"
chown teamdir-ana:teamdir-ana "$p/inbox/from-ana.txt"; chmod 0644 "$p/inbox/from-ana.txt"
chown teamdir-ben:teamdir-ben "$p/inbox/from-ben.txt"; chmod 0664 "$p/inbox/from-ben.txt"

# What the guard compares against: the regular files and their content.
(cd "$p" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum) > "$base/.manifest"

chown candidate:candidate "$base"
