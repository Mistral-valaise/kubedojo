#!/usr/bin/env bash
# expect: fail rest
# Starts the delete block one line too early, on the comment "# green pool":
# that line loses its comment sign and becomes a broken directive. The block
# is seven lines high, so all six servers are still activated.
set -euo pipefail
vim -es -N -u NONE \
  -c 'execute "normal! /server blue-1\<CR>^\<C-v>5jI# \<Esc>"' \
  -c 'execute "normal! /# green pool\<CR>^\<C-v>6jlx"' \
  -c 'execute "normal! /10\\.20\\.1\\.0\<CR>^\<C-v>5jIacl admin_net src \<Esc>"' \
  -c 'wq' /opt/course/vim-block-edit/haproxy.cfg
