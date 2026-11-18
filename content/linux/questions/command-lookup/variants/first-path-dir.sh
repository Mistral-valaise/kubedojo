#!/usr/bin/env bash
# expect: fail winner candidates
# Reads PATH by eye and looks into the directories with ls: the first
# directory that contains a deployctl is taken as the winner. The copy in
# scratch/bin is not executable and never runs.
set -euo pipefail
shopt -s expand_aliases
source /opt/course/command-lookup/team-env.sh
out=/opt/course/command-lookup
for n in rollout stage kill deployctl; do echo "$n $(type -t "$n")"; done > "$out/kinds.txt"
IFS=: read -ra dirs <<< "$PATH"
for d in "${dirs[@]}"; do
  if [ -e "$d/deployctl" ]; then echo "$d/deployctl"; fi
done > "$out/candidates.txt"
head -n 1 "$out/candidates.txt" > "$out/winner.txt"
