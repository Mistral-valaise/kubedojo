set -e
cd /opt/course/badger/app
sed -i -e 's|^ENV APP_TIER=.*|ENV APP_TIER=production|' Dockerfile
grep -q '^USER ' Dockerfile || sed -i 's|^CMD |USER 10001\n\nCMD |' Dockerfile
podman build -t badger-api:1.4.0 .
rm -f /opt/course/badger/badger-api.tar
podman save -o /opt/course/badger/badger-api.tar badger-api:1.4.0
