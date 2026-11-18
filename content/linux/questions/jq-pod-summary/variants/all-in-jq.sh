#!/usr/bin/env bash
# expect: pass
# Another correct way: selection, sum, ranking and de-duplication all inside
# jq (IN, map, sort_by, unique), without sort, uniq or head.
set -euo pipefail
cd /opt/course/jq-pod-summary
jq -r '.items[] | select(.status.phase | IN("Running", "Succeeded") | not)
  | .metadata.namespace + "/" + .metadata.name' pods.json > not-running.txt
jq -r '[.items[] | {pod: (.metadata.namespace + "/" + .metadata.name),
                    n: ((.status.containerStatuses // []) | map(.restartCount) | add // 0)}]
  | sort_by(-.n) | .[:3][] | "\(.n) \(.pod)"' pods.json > restarts.txt
jq -r '[.items[].spec.containers[].image] | unique | .[]' pods.json > images.txt
