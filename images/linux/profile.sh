# kubedojo shell defaults for the Linux lab host
export EDITOR=vim
export LANG=${LANG:-C.UTF-8}
if [ -n "${BASH_VERSION:-}" ] && [ -n "${PS1:-}" ]; then
  [ -f /usr/share/bash-completion/bash_completion ] && . /usr/share/bash-completion/bash_completion
  PS1='\[\e[1;32m\]➜ \u@\h\[\e[0m\]:\[\e[1;34m\]\w\[\e[0m\]\$ '
fi
