# expect: fail sudo-rule sudo-denied
# Restricts sudo to the binary, not to the command: every systemctl
# subcommand is still allowed.
set -e
ssh ${KD_HOST}-worker '
printf "svc-backup ALL=(root) NOPASSWD: /usr/bin/systemctl\n" > /etc/sudoers.d/svc-backup
chmod 440 /etc/sudoers.d/svc-backup
usermod -L jmiller
usermod -s /usr/sbin/nologin jmiller
rm -f /home/jmiller/.ssh/authorized_keys'
