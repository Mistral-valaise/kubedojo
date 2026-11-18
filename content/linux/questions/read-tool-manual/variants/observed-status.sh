#!/usr/bin/env bash
# expect: fail report status
# Runs the tool with its defaults plus the format, and takes the exit status
# it happens to return here for the answer: no --all, no --sort, and status 1
# means "a release differs", not "no MANIFEST".
cd /opt/course/read-tool-manual
bin/relcheck --format tsv releases > report.tsv
echo $? > no-manifest-status.txt
RELCHECK_IGNORE='*.bak:*.tmp' bin/relcheck --all --format tsv --sort size releases > report-clean.tsv
exit 0
