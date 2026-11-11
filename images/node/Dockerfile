# kubedojo node image: a real kubeadm node (kindest/node) made exam-like:
# sshd, a `candidate` user with passwordless sudo, etcdctl and common tools.
ARG K8S_VERSION=v1.37.0
FROM kindest/node:${K8S_VERSION}

ARG TARGETARCH
ARG ETCD_VERSION=v3.6.16
ARG HELM_VERSION=v3.22.1
ARG KUSTOMIZE_VERSION=v5.8.3
ARG TRIVY_VERSION=0.76.2

RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      openssh-server sudo vim less jq curl ca-certificates bash-completion \
      iproute2 iputils-ping dnsutils netcat-openbsd procps psmisc openssl \
 && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://github.com/etcd-io/etcd/releases/download/${ETCD_VERSION}/etcd-${ETCD_VERSION}-linux-${TARGETARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin --strip-components=1 \
          "etcd-${ETCD_VERSION}-linux-${TARGETARCH}/etcdctl" \
          "etcd-${ETCD_VERSION}-linux-${TARGETARCH}/etcdutl"

# Tools the candidate uses directly on the exam hosts.
RUN curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin --strip-components=1 "linux-${TARGETARCH}/helm" \
 && curl -fsSL "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F${KUSTOMIZE_VERSION}/kustomize_${KUSTOMIZE_VERSION}_linux_${TARGETARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin kustomize \
 && TRIVY_ARCH=$([ "$TARGETARCH" = "arm64" ] && echo ARM64 || echo 64bit) \
 && curl -fsSL "https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_Linux-${TRIVY_ARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin trivy

RUN useradd -m -s /bin/bash candidate \
 && echo 'candidate ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/candidate \
 && chmod 0440 /etc/sudoers.d/candidate \
 && install -d -m 0700 -o candidate -g candidate /home/candidate/.ssh \
 && install -d -m 0700 /root/.ssh \
 && systemctl enable ssh

COPY sshd_kubedojo.conf /etc/ssh/sshd_config.d/kubedojo.conf
COPY profile.sh /etc/profile.d/kubedojo.sh
COPY kubedojo-node-init /usr/local/bin/kubedojo-node-init
RUN chmod 0755 /usr/local/bin/kubedojo-node-init \
 && echo '. /etc/profile.d/kubedojo.sh' >> /etc/bash.bashrc
