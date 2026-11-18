#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/split-output-streams
# The tool exits with 3: catch the code instead of letting `set -e` stop here.
rc=0
bin/inventory-export > export.csv 2> warnings.log || rc=$?
echo "$rc" > exit-code.txt
