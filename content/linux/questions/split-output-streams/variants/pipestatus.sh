#!/usr/bin/env bash
# expect: pass
# Another correct way: watch the output through tee while saving it, and take
# the exit code of the tool from PIPESTATUS instead of $? (which is tee's).
cd /opt/course/split-output-streams
bin/inventory-export 2> warnings.log | tee export.csv > /dev/null
echo "${PIPESTATUS[0]}" > exit-code.txt
