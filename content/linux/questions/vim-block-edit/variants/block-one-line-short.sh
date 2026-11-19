#!/usr/bin/env bash
# expect: fail commented
# 4j instead of 5j: the block covers five of the six blue servers, and
# blue-6 stays live.
set -euo pipefail
vim -es -N -u NONE \
  -c 'execute "normal! /server blue-1\<CR>^\<C-v>4jI# \<Esc>"' \
  -c 'execute "normal! /# server green-1\<CR>^\<C-v>5jlx"' \
  -c 'execute "normal! /10\\.20\\.1\\.0\<CR>^\<C-v>5jIacl admin_net src \<Esc>"' \
  -c 'wq' /opt/course/vim-block-edit/haproxy.cfg
