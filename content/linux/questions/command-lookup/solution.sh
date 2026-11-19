#!/usr/bin/env bash
set -euo pipefail
# A script does not expand aliases unless told to; an interactive shell does.
shopt -s expand_aliases
source /opt/course/command-lookup/team-env.sh
cd /opt/course/command-lookup
for n in rollout stage kill deployctl; do
  echo "$n $(type -t "$n")"
done > kinds.txt
type -P deployctl > winner.txt
type -aP deployctl > candidates.txt
