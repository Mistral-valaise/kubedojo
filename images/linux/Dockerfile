# kubedojo linux image: the lab host of tracks that teach the shell. A plain
# Debian with the tools of an exam host and their man pages; no Kubernetes and
# no systemd. The learner reaches it from the terminal with `ssh <host>`.
#
# Not the slim variant: that one ships without man pages, and reading them is
# part of what the track teaches.
FROM debian:trixie

# Raise this whenever the image gets new contents; see images/node/Dockerfile.
LABEL dev.kubedojo.image-rev="1"

RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      bash bash-completion vim gawk sed grep findutils less \
      tar gzip bzip2 xz-utils zip unzip rsync \
      lsof strace acl bc file tree jq curl wget ca-certificates \
      dnsutils iproute2 iputils-ping netcat-openbsd procps psmisc \
      python3 cron nginx-light openssh-server openssh-client sudo \
      man-db manpages \
 && rm -rf /var/lib/apt/lists/* /etc/ssh/ssh_host_*

# Fail the build, not an exercise, when something the questions rely on is gone.
RUN man -w ls grep find tar awk sed vim ssh >/dev/null \
 && python3 -c 'import http.server'

RUN useradd -m -s /bin/bash candidate \
 && echo 'candidate ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/candidate \
 && chmod 0440 /etc/sudoers.d/candidate \
 && install -d -m 0755 -o candidate -g candidate /opt/course \
 && printf '%s\n' 'PasswordAuthentication no' 'PermitRootLogin prohibit-password' 'PubkeyAuthentication yes' \
      'UseDNS no' 'PrintMotd no' 'AcceptEnv LANG LC_*' > /etc/ssh/sshd_config.d/kubedojo.conf

COPY entrypoint.sh /usr/local/bin/kubedojo-linux-init
COPY kd-fixture /usr/local/bin/kd-fixture
COPY profile.sh /etc/profile.d/kubedojo.sh
COPY vimrc /home/candidate/.vimrc
COPY vimrc /root/.vimrc
# The build context may come out of the binary, where files carry no mode.
RUN chmod 0755 /usr/local/bin/kubedojo-linux-init /usr/local/bin/kd-fixture \
 && echo '. /etc/profile.d/kubedojo.sh' >> /etc/bash.bashrc \
 && chown candidate:candidate /home/candidate/.vimrc

ENV LANG=C.UTF-8
ENTRYPOINT ["kubedojo-linux-init"]
