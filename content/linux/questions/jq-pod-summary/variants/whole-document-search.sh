#!/usr/bin/env bash
# expect: fail not-running images
# Two shortcuts: "not Running" also reports the finished Job Pod, and a
# recursive search for .image finds the init container's image as well.
set -euo pipefail
cd /opt/course/jq-pod-summary
jq -r '.items[] | select(.status.phase != "Running")
  | "\(.metadata.namespace)/\(.metadata.name)"' pods.json > not-running.txt
jq -r '.items[] | "\([.status.containerStatuses[]?.restartCount] | add // 0) \(.metadata.namespace)/\(.metadata.name)"' pods.json \
  | sort -rn | head -3 > restarts.txt
jq -r '.. | .image? // empty' pods.json | sort -u > images.txt
