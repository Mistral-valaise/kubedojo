#!/usr/bin/env bash
# expect: fail stdout stderr
# Sends both streams into one file and copies it: neither file holds one
# stream only. The exit code is right.
cd /opt/course/split-output-streams
bin/inventory-export > export.csv 2>&1
echo $? > exit-code.txt
cp export.csv warnings.log
