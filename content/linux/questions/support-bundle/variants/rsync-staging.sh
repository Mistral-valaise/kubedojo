#!/usr/bin/env bash
# expect: pass
# Another correct way: rsync -a with excludes and cp -p into a staging tree
# next to the application, awk for the redaction, and members named ./app/...
set -euo pipefail
cd /opt/course/support-bundle
rm -rf stage && mkdir -p stage/app/log stage/app/bin
rsync -a --exclude='*.key' --exclude='*.env' app/etc stage/app/
cp -p app/log/app.log app/log/app.log.1 stage/app/log/
cp -p app/bin/run.sh stage/app/bin/
awk '/^db_password/ {print "db_password = REDACTED"; next} {print}' app/etc/app.conf > stage/app/etc/app.conf
(cd stage && tar -cf - .) | gzip -9 > bundle.tar.gz
rm -rf stage
