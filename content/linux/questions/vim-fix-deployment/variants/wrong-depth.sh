#!/usr/bin/env bash
# expect: fail port resources rest
# The values are fixed, but the new lines are added at the depth of
# "containers": valid YAML in which ports and resources belong to the Pod.
set -euo pipefail
vim -es -N -u NONE \
  -c '%s/^  replicas: 1$/  replicas: 3/' \
  -c '%s/ngnix/nginx/' \
  -c '/^      volumes:/-1' \
  -c 'call append(line("."), ["      ports:", "        - containerPort: 8080", "      resources:", "        requests:", "          cpu: 100m", "          memory: 64Mi"])' \
  -c 'wq' /opt/course/vim-fix-deployment/deployment.yaml
