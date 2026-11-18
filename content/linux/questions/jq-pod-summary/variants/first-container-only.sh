#!/usr/bin/env bash
# expect: fail restarts
# Reads the restart count of the first container only: the sidecars are
# ignored, so the numbers are too low and the third place goes to the wrong Pod.
set -euo pipefail
cd /opt/course/jq-pod-summary
jq -r '.items[] | select(.status.phase != "Running" and .status.phase != "Succeeded")
  | "\(.metadata.namespace)/\(.metadata.name)"' pods.json > not-running.txt
jq -r '.items[] | "\(.status.containerStatuses[0].restartCount // 0) \(.metadata.namespace)/\(.metadata.name)"' pods.json \
  | sort -rn | head -3 > restarts.txt
jq -r '.items[].spec.containers[].image' pods.json | sort -u > images.txt
