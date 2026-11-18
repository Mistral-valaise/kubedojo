# expect: fail archive
# The archive is saved from a first build, then the Dockerfile is fixed and the
# image rebuilt without saving again: the archive still holds the old image.
set -e
cd /opt/course/badger/app
podman build -t badger-api:1.4.0 .
podman save -o /opt/course/badger/badger-api.tar badger-api:1.4.0
sed -i -e 's|^ENV APP_TIER=.*|ENV APP_TIER=production|' Dockerfile
sed -i 's|^CMD |USER 10001\n\nCMD |' Dockerfile
podman build -t badger-api:1.4.0 .
