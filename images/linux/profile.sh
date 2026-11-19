# kubedojo shell defaults for the Linux lab host
export EDITOR=vim
# Debian keeps `service` and other admin tools in sbin, which a normal user's
# PATH lacks: "command not found" would hide the real answer, "permission denied".
case ":$PATH:" in *:/usr/sbin:*) ;; *) PATH="$PATH:/usr/sbin:/sbin" ;; esac
export LANG=${LANG:-C.UTF-8}
if [ -n "${BASH_VERSION:-}" ] && [ -n "${PS1:-}" ]; then
  [ -f /usr/share/bash-completion/bash_completion ] && . /usr/share/bash-completion/bash_completion
  PS1='\[\e[1;32m\]➜ \u@\h\[\e[0m\]:\[\e[1;34m\]\w\[\e[0m\]\$ '
fi
