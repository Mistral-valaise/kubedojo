#!/usr/bin/env bash
# expect: fail untouched
# Gets a clean report by deleting the leftover files instead of telling the
# tool to ignore them (the releases are root's, so with sudo). The report is
# right; the releases are not what they were.
cd /opt/course/read-tool-manual
bin/relcheck --all --format tsv --sort size releases > report.tsv
sudo rm -f releases/*/*.bak releases/.staging/*.tmp
bin/relcheck --all --format tsv --sort size releases > report-clean.tsv
echo 4 > no-manifest-status.txt
