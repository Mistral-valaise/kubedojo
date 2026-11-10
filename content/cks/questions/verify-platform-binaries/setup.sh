mkdir -p /opt/course/binaries && cd /opt/course/binaries
rm -f /opt/course/binaries/tampered
for n in a b c; do cp /usr/bin/kubectl /opt/course/binaries/kubectl-$n; done
printf '\001' >> /opt/course/binaries/kubectl-b
sha512sum /usr/bin/kubectl | cut -d' ' -f1 > /opt/course/binaries/kubectl.sha512
chown -R candidate:candidate /opt/course/binaries
