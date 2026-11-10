```bash
ssh {{host}}
trivy image --format spdx-json --output /opt/course/sbom/nginx.spdx.json nginx:1.27-alpine
jq '.spdxVersion, (.packages | length)' /opt/course/sbom/nginx.spdx.json
# "SPDX-2.3"
# 70
```

Query it:

```bash
jq -r '.packages[] | select(.name=="zlib") | .versionInfo' /opt/course/sbom/nginx.spdx.json
# 1.3.1-r2
jq -r '.packages[] | select(.name=="zlib") | .versionInfo' /opt/course/sbom/nginx.spdx.json \
  > /opt/course/sbom/zlib-version
```

(Write the version **your** SBOM shows.)

**Related commands:**

```bash
trivy image --format cyclonedx --output sbom.cdx.json <image>     # CycloneDX instead of SPDX
trivy sbom sbom.cdx.json                                          # scan an SBOM for CVEs
trivy image --severity HIGH,CRITICAL <image>                      # scan the image directly
```

Generating an SBOM does not need the vulnerability database, so it is fast; scanning for CVEs downloads
the database first.
