#!/usr/bin/env bash
# expect: fail exit-code
# Both streams are captured correctly, but a command runs before $? is read:
# the file holds the exit code of ls, which is 0.
cd /opt/course/split-output-streams
bin/inventory-export > export.csv 2> warnings.log
ls -l export.csv warnings.log > /dev/null
echo $? > exit-code.txt
