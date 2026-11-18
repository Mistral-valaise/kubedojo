# expect: pass
# An OCI archive is as valid as the default docker-archive, and `USER uid:gid`
# as valid as `USER uid`.
set -e
cd /opt/course/badger/app
sed -i -e 's|^ENV APP_TIER=.*|ENV APP_TIER="production"|' Dockerfile
echo 'USER 10001:10001' >> Dockerfile
podman build -t badger-api:1.4.0 -f Dockerfile /opt/course/badger/app
rm -f /opt/course/badger/badger-api.tar
podman save --format oci-archive -o /opt/course/badger/badger-api.tar localhost/badger-api:1.4.0
