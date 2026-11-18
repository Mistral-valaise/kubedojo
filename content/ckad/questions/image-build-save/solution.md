```bash
ssh {{host}}
cd /opt/course/badger/app
vim Dockerfile
```

```dockerfile
FROM docker.io/library/busybox:1.36

LABEL org.opencontainers.image.title="badger-api"
ENV APP_TIER=production

WORKDIR /srv
COPY motd.txt /srv/motd.txt

USER 10001

CMD ["cat", "/srv/motd.txt"]
```

`USER` applies to everything below it and to the running container, so it goes after the steps that
need root (here there are none) and before `CMD`.

```bash
podman build -t badger-api:1.4.0 .                 # the dot is the build context
podman image ls badger-api                         # localhost/badger-api  1.4.0
podman image inspect badger-api:1.4.0 --format '{{.Config.User}} {{.Config.Env}}'
# 10001 [PATH=... APP_TIER=production]

podman save -o /opt/course/badger/badger-api.tar badger-api:1.4.0
tar -tf /opt/course/badger/badger-api.tar          # layers, config, manifest.json
```

On this host `podman` runs as root; for `candidate` the shell turns `podman` into `sudo podman`, so the
image is in root's store either way.

**Takeaways:**

- Build, tag, save are three different things: `-t` names the image at build time, `podman tag` adds
  another name later, `podman save` writes whatever the name points to **now**. After a rebuild, save
  again.
- Without a registry in the name podman stores the image as `localhost/badger-api:1.4.0`.
- The exam may also ask for `podman push <registry>/<name>:<tag>` or for an OCI archive
  (`podman save --format oci-archive`). The flags are in `podman save --help`.
