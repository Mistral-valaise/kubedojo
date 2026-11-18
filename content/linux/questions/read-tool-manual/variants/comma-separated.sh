#!/usr/bin/env bash
# expect: fail clean
# Guesses the list separator instead of reading the manual: with a comma the
# value is one pattern that matches no file, so the "clean" report is the
# same as the first one.
cd /opt/course/read-tool-manual
bin/relcheck --all --format tsv --sort size releases > report.tsv
RELCHECK_IGNORE='*.bak,*.tmp' bin/relcheck --all --format tsv --sort size releases > report-clean.tsv
echo 4 > no-manifest-status.txt
