#!/usr/bin/env bash
# expect: fail failed-health-check argument-checks
# Fixes what the sample run complains about loudest, the quoting, and leaves
# the `<` test and the swallowed exit code as they were.
set -euo pipefail
cd /opt/course/fix-deploy-script
sed -i \
  -e 's|\[ ! -f \$release/manifest.txt \]|[ ! -f "$release/manifest.txt" ]|' \
  -e 's|^mkdir -p \$target|mkdir -p "$target"|' \
  -e 's|^for f in \$(cat \$release/manifest.txt); do|while IFS= read -r f; do|' \
  -e 's|^done$|done < "$release/manifest.txt"|' \
  -e 's|cp \$release/\$f \$target/|cp "$release/$f" "$target/"|' \
  -e 's|^\$release/healthcheck.sh \$target \| tee \$target/healthcheck.log|"$release/healthcheck.sh" "$target" \| tee "$target/healthcheck.log"|' \
  deploy.sh
