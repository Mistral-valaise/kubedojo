#!/usr/bin/env bash
# expect: fail env resources volumes
# Converts the tabs and stops: the file is valid YAML again, but env and
# resources are fields of the Pod and volumes hangs below requests.
set -euo pipefail
vim -es -N -u NONE -c 'set tabstop=2 expandtab' -c 'retab' -c 'wq' /opt/course/vim-yaml-indent/pod.yaml
