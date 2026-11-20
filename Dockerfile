# kubedojo server image (hosted mode). Local users run the release binary.
FROM node:26-alpine AS web
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

FROM alpine:3.24
ARG TARGETARCH
ARG KIND_VERSION=v0.33.0
# The checksums of kind ${KIND_VERSION}, from its release page. A new version
# needs new ones: the build fails on a download that does not match.
ARG KIND_SHA256_amd64=aee6151561422756b764a4ae28e7f44cda5af5a9eead3cc9985112b1de8d8e0d
ARG KIND_SHA256_arm64=20022bee6cfcd5086cb7234d218e3454e6090022f2a8f55d1fa7fcf42c3867a2
RUN apk add --no-cache ca-certificates docker-cli tini \
 && wget -qO /usr/local/bin/kind "https://github.com/kubernetes-sigs/kind/releases/download/${KIND_VERSION}/kind-linux-${TARGETARCH}" \
 && eval "sum=\$KIND_SHA256_${TARGETARCH}" \
 && echo "${sum}  /usr/local/bin/kind" | sha256sum -c - \
 && chmod 0755 /usr/local/bin/kind \
 && adduser -D -u 10001 kubedojo && mkdir -p /data && chown kubedojo /data
COPY --from=build /kubedojo /usr/local/bin/kubedojo
USER 10001
ENV KUBEDOJO_DATA=/data KUBEDOJO_ADDR=0.0.0.0:4400
EXPOSE 4400
ENTRYPOINT ["/sbin/tini", "--", "kubedojo"]
CMD ["serve"]
