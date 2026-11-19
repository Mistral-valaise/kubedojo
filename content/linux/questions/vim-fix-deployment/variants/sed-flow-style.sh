#!/usr/bin/env bash
# expect: pass
# sed instead of an editor, and another valid way of writing it: flow style
# on one line, a different key order, a named port, and comments.
set -euo pipefail
sed -i -E \
  -e 's/^(  replicas:).*/\1 3   # scaled up for the sale/' \
  -e 's|^( *)image: ngnix:1.27-alpine|\1resources: {requests: {memory: 64Mi, cpu: "100m"}}\n\1ports: [{name: http, containerPort: 8080}]\n\1image: nginx:1.27-alpine   # was misspelled|' \
  /opt/course/vim-fix-deployment/deployment.yaml
