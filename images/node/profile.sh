# kubedojo shell defaults (same conveniences the real exam environment offers)
alias k=kubectl
# Image builds need root on a node that is itself a container (no nested user namespaces).
if [ "$(id -u)" != 0 ]; then alias podman='sudo podman' buildah='sudo buildah'; fi
export EDITOR=vim
export do="--dry-run=client -o yaml"
export now="--force --grace-period 0"
if [ -n "${BASH_VERSION:-}" ] && command -v kubectl >/dev/null 2>&1; then
  [ -f /usr/share/bash-completion/bash_completion ] && . /usr/share/bash-completion/bash_completion
  source <(kubectl completion bash) 2>/dev/null
  complete -o default -F __start_kubectl k 2>/dev/null
fi
if [ -n "${PS1:-}" ]; then
  PS1='\[\e[1;32m\]➜ \u@\h\[\e[0m\]:\[\e[1;34m\]\w\[\e[0m\]\$ '
fi
