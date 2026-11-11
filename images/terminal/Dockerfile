# kubedojo terminal image: the `candidate@terminal` jump host the browser
# terminal attaches to. From here the candidate reaches every exam host via ssh.
FROM debian:trixie-slim

ARG TARGETARCH
ARG K8S_VERSION=v1.37.0
ARG HELM_VERSION=v3.22.1
ARG KUSTOMIZE_VERSION=v5.8.3
ARG YQ_VERSION=v4.54.3
ARG TRIVY_VERSION=0.76.2

RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      bash bash-completion openssh-client sudo vim nano tmux less jq curl wget git \
      ca-certificates iproute2 iputils-ping dnsutils netcat-openbsd procps openssl \
      man-db tree \
 && rm -rf /var/lib/apt/lists/*

RUN curl -fsSLo /usr/local/bin/kubectl "https://dl.k8s.io/release/${K8S_VERSION}/bin/linux/${TARGETARCH}/kubectl" \
 && curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin --strip-components=1 "linux-${TARGETARCH}/helm" \
 && curl -fsSL "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F${KUSTOMIZE_VERSION}/kustomize_${KUSTOMIZE_VERSION}_linux_${TARGETARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin kustomize \
 && curl -fsSLo /usr/local/bin/yq "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_${TARGETARCH}" \
 && TRIVY_ARCH=$([ "$TARGETARCH" = "arm64" ] && echo ARM64 || echo 64bit) \
 && curl -fsSL "https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_Linux-${TRIVY_ARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin trivy \
 && chmod 0755 /usr/local/bin/kubectl /usr/local/bin/helm /usr/local/bin/kustomize /usr/local/bin/yq /usr/local/bin/trivy

RUN useradd -m -s /bin/bash candidate \
 && echo 'candidate ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/candidate \
 && chmod 0440 /etc/sudoers.d/candidate \
 && install -d -m 0700 -o candidate -g candidate /home/candidate/.ssh /home/candidate/.kube

COPY profile.sh /etc/profile.d/kubedojo.sh
COPY vimrc /home/candidate/.vimrc
COPY tmux.conf /home/candidate/.tmux.conf
RUN echo '. /etc/profile.d/kubedojo.sh' >> /etc/bash.bashrc \
 && chown candidate:candidate /home/candidate/.vimrc /home/candidate/.tmux.conf

USER candidate
WORKDIR /home/candidate
ENV TERM=xterm-256color LANG=C.UTF-8
CMD ["sleep", "infinity"]
