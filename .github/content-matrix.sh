#!/usr/bin/env bash
# Prints the matrix of the "content" CI job as JSON: which tracks, and which
# of their questions, a change needs proven against a real cluster.
#
#   [{"track":"cka","questions":""},{"track":"ckad","questions":"a,b"}]
#
# An empty "questions" means the whole track. "[]" means nothing to prove.
#
# How much is proven depends on what changed:
#
#   everything  the grader, the catalog, the images: the grade of any question
#               may change. Also with the pull request label "content".
#   a sample    the session code, the sandbox, the pipeline: two questions of
#               every environment of every track show that sandboxes come up
#               and that questions are set up, solved and graded. The night
#               proves the rest (the job content-nightly).
#   changed     the questions a change touches, and a track whose track.yaml
#               changed.
#
#   BASE   commit to compare HEAD with (the pull request's base, or the commit
#          before a push). Unknown or missing: everything is proven.
#   ALL    "true" proves everything (the pull request carries the content label).
#   EVENT  the event that started the run. A push to main proves a sample at
#          most: its pull request has proven everything already.
set -euo pipefail

# Theory tracks (kind: theory) have no sandbox: the build job proves them.
tracks=$(for f in content/*/track.yaml; do
  grep -qE '^kind:[[:space:]]*theory[[:space:]]*$' "$f" || basename "$(dirname "$f")"
done)
everything() { for t in $tracks; do printf '%s\t\n' "$t"; done; }

# Two questions of every environment of a track, always the same ones.
sample() {
  for q in "content/$1"/questions/*/question.yaml; do
    printf '%s\t%s\n' "$(sed -nE 's/^environment:[[:space:]]*([^[:space:]#]+).*/\1/p' "$q" | head -n1)" "$(basename "$(dirname "$q")")"
  done | sort | awk -F'\t' 'seen[$1]++ < 2 { print $2 }'
}

rows() {
  if [ "${ALL:-}" = "true" ] || [ -z "${BASE:-}" ] || ! git cat-file -e "$BASE^{commit}" 2>/dev/null; then
    everything; return
  fi
  local changed code mode=changed
  changed=$(git diff --name-only "$BASE"...HEAD 2>/dev/null || git diff --name-only "$BASE" HEAD)
  # A test of the code proves itself in the build job.
  code=$(grep -vE '_test\.go$' <<<"$changed" || true)
  if grep -qE '^(images/|internal/(checker|catalog)/|cmd/kubedojo/test\.go$)' <<<"$code"; then
    mode=everything
  elif grep -qE '^(internal/(session|sandbox|dockerhost)/|cmd/kubedojo/(doctor|stubs)\.go$|Makefile$|\.github/(workflows/ci\.yaml|content-matrix\.sh)$)' <<<"$code"; then
    mode=sample
  fi
  if [ "$mode" = everything ] && [ "${EVENT:-}" = push ]; then mode=sample; fi
  if [ "$mode" = everything ]; then everything; return; fi

  for t in $tracks; do
    # A change to the track itself (domains, environments) concerns all its questions.
    if grep -qxF "content/$t/track.yaml" <<<"$changed"; then printf '%s\t\n' "$t"; continue; fi
    local ids
    # Only questions that still exist: a deleted one has nothing to prove.
    ids=$({
      sed -nE "s|^content/$t/questions/([^/]+)/.*|\1|p" <<<"$changed" \
        | while read -r id; do [ -d "content/$t/questions/$id" ] && echo "$id"; done
      if [ "$mode" = sample ]; then sample "$t"; fi
    } | sort -u | paste -sd, -)
    [ -n "$ids" ] && printf '%s\t%s\n' "$t" "$ids"
  done
  return 0
}

rows | jq -Rnc '[inputs | split("\t") | {track: .[0], questions: (.[1] // "")}]'
