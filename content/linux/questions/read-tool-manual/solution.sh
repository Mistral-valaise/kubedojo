#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/read-tool-manual
# relcheck exits 1 because some releases are not OK: that is its result, not a failure.
bin/relcheck --all --format tsv --sort size releases > report.tsv || [ $? -eq 1 ]
RELCHECK_IGNORE='*.bak:*.tmp' bin/relcheck --all --format tsv --sort size releases > report-clean.tsv || [ $? -eq 1 ]
echo 4 > no-manifest-status.txt
