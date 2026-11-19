#!/usr/bin/env bash
# expect: pass
# Another correct way: read the sentence `type` prints, take the winner from
# `command -v` and the list from `which -a`, which also skips files that are
# not executable.
set -euo pipefail
shopt -s expand_aliases
source /opt/course/command-lookup/team-env.sh
out=/opt/course/command-lookup
for n in rollout stage kill deployctl; do
  case "$(type "$n")" in
    *"is aliased to"*)     echo "$n alias" ;;
    *"is a function"*)     echo "$n function" ;;
    *"is a shell builtin") echo "$n builtin" ;;
    *)                     echo "$n file" ;;
  esac
done > "$out/kinds.txt"
command -v deployctl > "$out/winner.txt"
which -a deployctl > "$out/candidates.txt"
