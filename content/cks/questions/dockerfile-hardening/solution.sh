set -e
f=/opt/course/docker/Dockerfile
sed -i -e 's|^FROM python:latest|FROM python:3.13-slim|' -e '/^ENV API_TOKEN=/d' -e 's|^USER root|USER app|' "$f"
