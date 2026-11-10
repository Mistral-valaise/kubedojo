```bash
ssh {{host}}
vim /opt/course/docker/Dockerfile
```

```dockerfile
FROM python:3.13-slim

ENV PORT=8080

RUN useradd --create-home --uid 10001 app
WORKDIR /srv/api
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .

USER app
EXPOSE 8080
CMD ["python", "main.py"]
```

Three changes:

| Before | After | Why |
|---|---|---|
| `FROM python:latest` | `FROM python:3.13-slim` | reproducible builds, smaller image, fewer CVEs |
| `ENV API_TOKEN=...` | *(line removed)* | image layers are readable by anyone who can pull |
| `USER root` | `USER app` | a container escape should not start as root |

In the cluster the token would come from a Secret (file or `secretKeyRef`), and the Pod would add
`runAsNonRoot: true` so Kubernetes refuses to start the image if someone reverts the `USER` line.

**Further hardening you may be asked for:** multi-stage builds (compilers stay in the build stage),
distroless or scratch runtime images, pinning by digest, and removing shells and package managers.
