#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/jq-pod-summary

jq -r '.items[]
  | select(.status.phase != "Running" and .status.phase != "Succeeded")
  | "\(.metadata.namespace)/\(.metadata.name)"' pods.json > not-running.txt

jq -r '.items[]
  | "\([.status.containerStatuses[]?.restartCount] | add // 0) \(.metadata.namespace)/\(.metadata.name)"' pods.json \
  | sort -rn | head -3 > restarts.txt

jq -r '.items[].spec.containers[].image' pods.json | sort -u > images.txt
