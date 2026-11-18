d=/opt/course/badger
podman rmi -f localhost/badger-api:1.4.0 >/dev/null 2>&1 || true
rm -rf "$d" && mkdir -p "$d/app"
cat > "$d/app/Dockerfile" <<'DOCKER'
FROM docker.io/library/busybox:1.36

LABEL org.opencontainers.image.title="badger-api"
ENV APP_TIER=staging

WORKDIR /srv
COPY motd.txt /srv/motd.txt

CMD ["cat", "/srv/motd.txt"]
DOCKER
echo "badger-api: ready to serve" > "$d/app/motd.txt"
chown -R candidate:candidate "$d"

# Put the base image into podman's store from the node's containerd (which
# pulls through the local cache), so that the build needs no registry.
if ! podman image exists docker.io/library/busybox:1.36 2>/dev/null; then
  t=$(mktemp -d)
  crictl pull docker.io/library/busybox:1.36 >/dev/null 2>&1 \
    && ctr -n k8s.io images export --platform "linux/$(dpkg --print-architecture)" "$t/busybox.tar" docker.io/library/busybox:1.36 >/dev/null 2>&1 \
    && podman load -i "$t/busybox.tar" >/dev/null 2>&1 || true
  rm -rf "$t"
fi
