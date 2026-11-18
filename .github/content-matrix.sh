#!/usr/bin/env bash
# Prints the matrix of the "content" CI job as JSON: which tracks, and which
# of their questions, a change needs proven against a real cluster.
#
#   [{"track":"cka","questions":""},{"track":"ckad","questions":"a,b"}]
#
# An empty "questions" means the whole track. "[]" means nothing to prove.
#
#   BASE  commit to compare HEAD with (the pull request's base, or the commit
#         before a push). Unknown or missing: everything is proven.
#   ALL   "true" proves everything (the pull request carries the content label).
set -euo pipefail

# Theory tracks (kind: theory) have no sandbox: the build job proves them.
tracks=$(for f in content/*/track.yaml; do
  grep -qE '^kind:[[:space:]]*theory[[:space:]]*$' "$f" || basename "$(dirname "$f")"
done)
everything() { for t in $tracks; do printf '%s\t\n' "$t"; done; }

rows() {
  if [ "${ALL:-}" = "true" ] || [ -z "${BASE:-}" ] || ! git cat-file -e "$BASE^{commit}" 2>/dev/null; then
    everything; return
  fi
  local changed
  changed=$(git diff --name-only "$BASE"...HEAD 2>/dev/null || git diff --name-only "$BASE" HEAD)
  # The grader, the sandbox, the images or this pipeline: any question may be affected.
  if grep -qE '^(images/|internal/(checker|catalog|session|sandbox)/|cmd/kubedojo/(test|doctor)\.go$|Makefile$|\.github/(workflows/ci\.yaml|content-matrix\.sh)$)' <<<"$changed"; then
    everything; return
  fi
  for t in $tracks; do
    # A change to the track itself (domains, environments) concerns all its questions.
    if grep -qxF "content/$t/track.yaml" <<<"$changed"; then printf '%s\t\n' "$t"; continue; fi
    # Only questions that still exist: a deleted one has nothing to prove.
    local ids
    ids=$(sed -nE "s|^content/$t/questions/([^/]+)/.*|\1|p" <<<"$changed" | sort -u \
      | while read -r id; do [ -d "content/$t/questions/$id" ] && echo "$id"; done | paste -sd, -)
    [ -n "$ids" ] && printf '%s\t%s\n' "$t" "$ids"
  done
  return 0
}

rows | jq -Rnc '[inputs | split("\t") | {track: .[0], questions: (.[1] // "")}]'
