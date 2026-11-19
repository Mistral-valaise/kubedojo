#!/usr/bin/env bash
# expect: fail bad-calls
# The loop and the exit code are right, but nothing is validated: a missing
# list makes the redirection fail with exit code 1, which reads as "a service
# is down".
set -euo pipefail
cat > /opt/course/service-status-report/check-services.sh <<'SCRIPT'
#!/usr/bin/env bash
failed=0
while read -r name host port || [ -n "$name" ]; do
  case $name in ''|'#'*) continue ;; esac
  if nc -z -w 2 "$host" "$port" 2>/dev/null; then
    echo "$name UP"
  else
    echo "$name DOWN"
    failed=1
  fi
done < "$1" > "$2"
exit "$failed"
SCRIPT
chmod +x /opt/course/service-status-report/check-services.sh
