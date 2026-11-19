#!/usr/bin/env bash
# expect: fail hardlinks count
# Takes "the same file" for "a link to the file": symbolic links in the
# releases, and the link count read off the number of releases instead of
# from the inode.
set -euo pipefail
cd /opt/course/release-links/app
ln -sfn releases/2024-05-20 current
ln -sfn releases/2024-04-15 previous
ln -sf ../../shared/license.key releases/2024-04-15/license.key
ln -sf ../../shared/license.key releases/2024-05-20/license.key
echo 4 > ../link-count.txt
