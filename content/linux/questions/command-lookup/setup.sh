#!/usr/bin/env bash
# Three files called deployctl in three PATH directories, plus an alias and a
# function, all brought in by a file the learner sources. scratch/bin comes
# first in PATH but its deployctl is not executable, so legacy/bin wins.
set -euo pipefail

base=/opt/course/command-lookup
rm -rf "$base"
mkdir -p "$base/legacy/bin" "$base/tools/bin" "$base/scratch/bin"

# mk <dir> <version> <mode>
mk() {
  printf '#!/bin/sh\necho "deployctl %s $*"\n' "$2" > "$base/$1/bin/deployctl"
  chmod "$3" "$base/$1/bin/deployctl"
}
mk legacy  "1.4 (legacy)" 755
mk tools   "2.0"          755
mk scratch "3.0-dev"      644

cat > "$base/team-env.sh" <<'ENV'
# Shell environment of the platform team. Load it with:  source team-env.sh
PATH=/opt/course/command-lookup/scratch/bin:/opt/course/command-lookup/legacy/bin:/opt/course/command-lookup/tools/bin:$PATH
alias rollout='deployctl --env prod'
stage() { deployctl --env stage "$@"; }
ENV

(cd "$base" && { sha256sum team-env.sh; find legacy tools scratch -printf '%y %m %s %p\n' | LC_ALL=C sort; } \
  | sha256sum | cut -d' ' -f1) > "$base/.manifest"

# The learner writes the result files; the fixtures stay root's.
chown candidate:candidate "$base"
