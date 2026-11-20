#!/bin/sh
set -eu

# Keep the sandbox inside its Pod's limits.
#
# A privileged container shares the cgroup namespace of the node. Docker would
# create the cgroups of its containers at the root of the node's hierarchy,
# next to the Pods and not inside this one: the clusters of a session would not
# count against the Pod's memory and CPU limits, and Kubernetes would see only
# the Docker daemon. So Docker is told to put its containers below this
# container's own cgroup.
#
# cgroup v2 lets a cgroup have either processes or children with controllers,
# not both. The processes of this container therefore move to a leaf, "init",
# before the controllers are handed down to "docker".
cgroup_parent=""
if [ -f /sys/fs/cgroup/cgroup.controllers ]; then
  own=$(sed -n 's/^0:://p' /proc/self/cgroup)
  base="/sys/fs/cgroup$own"
  # "/" is a private cgroup namespace: the image's own setup handles that.
  if [ -n "$own" ] && [ "$own" != "/" ] && [ -d "$base" ]; then
    mkdir -p "$base/init"
    # Repeat: a process may start while the others are being moved.
    for _ in 1 2 3; do
      while read -r pid; do
        echo "$pid" > "$base/init/cgroup.procs" 2>/dev/null || true
      done < "$base/cgroup.procs"
    done
    for c in $(cat "$base/cgroup.controllers"); do
      echo "+$c" > "$base/cgroup.subtree_control" 2>/dev/null || true
    done
    if grep -qw memory "$base/cgroup.subtree_control"; then
      cgroup_parent="$own/docker"
      echo "sandbox containers are accounted to this Pod: cgroup parent $cgroup_parent"
    else
      echo "warning: could not delegate cgroup controllers below $own;" \
        "the containers of this sandbox do not count against the Pod's limits" >&2
    fi
  fi
else
  echo "warning: cgroup v1: the containers of this sandbox do not count against the Pod's limits" >&2
fi

dockerd-entrypoint.sh dockerd --host=unix:///var/run/docker.sock --host=tcp://0.0.0.0:2375 \
  ${cgroup_parent:+--cgroup-parent="$cgroup_parent"} &
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
