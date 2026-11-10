#!/bin/sh
set -eu
dockerd-entrypoint.sh dockerd --host=unix:///var/run/docker.sock --host=tcp://0.0.0.0:2375 &
pid=$!
i=0
until docker info >/dev/null 2>&1; do
  i=$((i + 1)); [ "$i" -gt 120 ] && { echo "dockerd did not start" >&2; exit 1; }
  sleep 1
done
for tar in /images/*.tar; do
  echo "loading $tar"
  docker load -q -i "$tar"
done
touch /run/kubedojo-ready
echo "sandbox host ready"
wait "$pid"
