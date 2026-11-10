mkdir -p /opt/course/docker
cat > /opt/course/docker/Dockerfile <<'DOCKER'
FROM python:latest

ENV API_TOKEN=tok_live_9f8a7c6b5d4e
ENV PORT=8080

RUN useradd --create-home --uid 10001 app
WORKDIR /srv/api
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .

USER root
EXPOSE 8080
CMD ["python", "main.py"]
DOCKER
chown -R candidate:candidate /opt/course/docker
