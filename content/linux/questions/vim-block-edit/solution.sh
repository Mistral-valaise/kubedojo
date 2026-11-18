#!/usr/bin/env bash
# The keystrokes of the worked solution, replayed with :normal. <C-v> is
# Ctrl-v (visual block), ^ the first non-blank column of the line.
set -euo pipefail
vim -es -N -u NONE \
  -c 'execute "normal! /server blue-1\<CR>^\<C-v>5jI# \<Esc>"' \
  -c 'execute "normal! /# server green-1\<CR>^\<C-v>5jlx"' \
  -c 'execute "normal! /10\\.20\\.1\\.0\<CR>^\<C-v>5jIacl admin_net src \<Esc>"' \
  -c 'wq' /opt/course/vim-block-edit/haproxy.cfg
