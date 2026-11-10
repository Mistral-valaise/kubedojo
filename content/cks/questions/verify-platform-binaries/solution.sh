set -e
cd /opt/course/binaries
want=$(cat kubectl.sha512)
: > tampered
for f in kubectl-a kubectl-b kubectl-c; do
  [ "$(sha512sum "$f" | cut -d' ' -f1)" = "$want" ] || { echo "$f" >> tampered; rm -f "/opt/course/binaries/$f"; }
done
