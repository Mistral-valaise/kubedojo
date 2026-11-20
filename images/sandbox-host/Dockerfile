# Sandbox host for hosted mode: one Docker-in-Docker daemon per session, with
# the kubedojo node, terminal and linux images preloaded so sessions start
# without pulling gigabytes from a registry.
#
# Build with `make sandbox-host` (it exports the three images next to this file).
# Pinned by digest: the entrypoint relies on how this image starts dockerd.
FROM docker:29.10.0-dind@sha256:4bbc7223f81db790097c6dea42fb93e05b8fead4f0431fbe6ccaba6e8473201c
COPY node.tar terminal.tar linux.tar /images/
COPY entrypoint.sh /usr/local/bin/kubedojo-sandbox-host
RUN chmod 0755 /usr/local/bin/kubedojo-sandbox-host
# The daemon is reachable only inside the cluster and fenced by a NetworkPolicy
# that admits the kubedojo server alone; see the Helm chart.
ENV DOCKER_TLS_CERTDIR=""
EXPOSE 2375
ENTRYPOINT ["kubedojo-sandbox-host"]
