#!/usr/bin/env bash
# A tree of site configurations that mention a database host. Seven files end
# in .conf (four of them with blanks or a quote in the name, one hidden); six
# mention the old host. Other files mention it too and must stay as they are.
set -euo pipefail

base=/opt/course/bulk-edit-spaced-names
rm -rf "$base"
mkdir -p "$base/sites/shop" "$base/sites/blog" "$base/sites/internal tools" "$base/sites/archive"
cd "$base/sites"

# conf <path> <site name>: a site configuration that uses the old database host.
conf() {
  cat > "$1" <<CONF
# site: $2
server_name = $2.example.org
database_url = postgres://app@db-old.internal:5432/$2
session_store = redis://cache.internal:6379/0
CONF
}

conf "shop/shop frontend.conf"          shop
conf "blog/My Blog (staging).conf"      blog-staging
conf "internal tools/wiki.conf"         wiki
conf "internal tools/O'Brien test.conf" obrien-test
conf "blog/.cache.conf"                 blog-cache

# Two occurrences on one line.
cat > shop/checkout.conf <<'CONF'
# site: checkout
server_name = checkout.example.org
database_hosts = db-old.internal:5432,db-old.internal:5433
session_store = redis://cache.internal:6379/1
CONF

# Not *.conf, or no mention of the old host: all of these must stay unchanged.
cat > blog/blog.conf <<'CONF'
# site: blog
server_name = blog.example.org
database_url = sqlite:///var/lib/blog/blog.db
CONF
cp shop/checkout.conf shop/checkout.conf.bak
conf "archive/2023 legacy.conf.disabled" legacy
cat > shop/README.txt <<'TXT'
History: until the migration all sites used db-old.internal.
Keep this note for the audit.
TXT
cat > "internal tools/migration notes.md" <<'TXT'
- [ ] point every site from db-old.internal to db-new.internal
TXT

# Expected state of the *.conf files, and the state of the files that must not change.
tmp=$(mktemp -d)
cp -a "$base/sites" "$tmp/sites"
(cd "$tmp/sites" && find . -type f -name '*.conf' -exec sed -i 's/db-old\.internal/db-new.internal/g' {} + \
  && find . -type f -name '*.conf' -print0 | LC_ALL=C sort -z | xargs -0 sha256sum) > "$base/.expected-conf"
rm -rf "$tmp"
find . -type f ! -name '*.conf' -print0 | LC_ALL=C sort -z | xargs -0 sha256sum > "$base/.manifest-others"
sha256sum blog/blog.conf >> "$base/.manifest-others"

# The learner edits files in place, which also needs write access to the directories.
chown -R candidate:candidate "$base/sites"
chown candidate:candidate "$base"
