set -e
trivy image --quiet --format spdx-json --output /opt/course/sbom/nginx.spdx.json nginx:1.27-alpine
jq -r '[.packages[] | select(.name=="zlib") | .versionInfo][0]' /opt/course/sbom/nginx.spdx.json > /opt/course/sbom/zlib-version
