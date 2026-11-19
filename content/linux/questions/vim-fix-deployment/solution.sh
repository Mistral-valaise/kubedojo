#!/usr/bin/env bash
# Two substitutes for the values, two new lines below the image, and the
# resources block of the other container copied (:t) and corrected.
set -euo pipefail
vim -es -N -u NONE \
  -c '%s/^  replicas: 1$/  replicas: 3/' \
  -c '%s/ngnix/nginx/' \
  -c 'call append(line("."), ["          ports:", "            - containerPort: 8080"])' \
  -c '/^          resources:/,/^              memory:/t.' \
  -c 's/16Mi/64Mi/' \
  -c '-1s/10m/100m/' \
  -c 'wq' /opt/course/vim-fix-deployment/deployment.yaml
