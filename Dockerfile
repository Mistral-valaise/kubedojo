# kubedojo server image (hosted mode). Local users run the release binary.
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --silent
COPY web/ ./
RUN npm run build -- --outDir /out --emptyOutDir

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /out ./internal/webui/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /kubedojo ./cmd/kubedojo

FROM alpine:3.22
ARG TARGETARCH
ARG KIND_VERSION=v0.33.0
RUN apk add --no-cache ca-certificates docker-cli tini \
 && wget -qO /usr/local/bin/kind "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-${TARGETARCH}" \
 && chmod 0755 /usr/local/bin/kind \
 && adduser -D -u 10001 kubedojo && mkdir -p /data && chown kubedojo /data
COPY --from=build /kubedojo /usr/local/bin/kubedojo
USER 10001
ENV KUBEDOJO_DATA=/data KUBEDOJO_ADDR=0.0.0.0:4400
EXPOSE 4400
ENTRYPOINT ["/sbin/tini", "--", "kubedojo"]
CMD ["serve"]
