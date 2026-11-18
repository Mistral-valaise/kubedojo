#!/usr/bin/env bash
# expect: fail kinds
# Asks `which` for everything. It finds /usr/bin/kill and calls the builtin a
# file, and it finds nothing for the alias and the function. For deployctl,
# a real file, its answers are right.
shopt -s expand_aliases
source /opt/course/command-lookup/team-env.sh
out=/opt/course/command-lookup
for n in rollout stage kill deployctl; do
  if which "$n" >/dev/null 2>&1; then echo "$n file"; else echo "$n function"; fi
done > "$out/kinds.txt"
which deployctl > "$out/winner.txt"
which -a deployctl > "$out/candidates.txt"
