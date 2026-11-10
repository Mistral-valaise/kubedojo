# Sandbox host for hosted mode: one Docker-in-Docker daemon per session, with
# the kubedojo node and terminal images preloaded so sessions start without
# pulling gigabytes from a registry.
#
# Build with `make sandbox-host` (it exports the two images next to this file).
FROM docker:29-dind
COPY node.tar terminal.tar /images/
COPY entrypoint.sh /usr/local/bin/kubedojo-sandbox-host
RUN chmod 0755 /usr/local/bin/kubedojo-sandbox-host
# The daemon is reachable only inside the cluster and fenced by a NetworkPolicy
# that admits the kubedojo server alone; see the Helm chart.
ENV DOCKER_TLS_CERTDIR=""
EXPOSE 2375
ENTRYPOINT ["kubedojo-sandbox-host"]
