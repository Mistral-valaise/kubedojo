#!/usr/bin/env bash
# expect: fail resources
# Copies the resources block of log-shipper to the right place and forgets to
# change the values.
set -euo pipefail
vim -es -N -u NONE \
  -c '%s/^  replicas: 1$/  replicas: 3/' \
  -c '%s/ngnix/nginx/' \
  -c 'call append(line("."), ["          ports:", "            - containerPort: 8080"])' \
  -c '/^          resources:/,/^              memory:/t.' \
  -c 'wq' /opt/course/vim-fix-deployment/deployment.yaml
