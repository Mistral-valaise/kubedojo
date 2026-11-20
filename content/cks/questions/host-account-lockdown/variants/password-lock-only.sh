# expect: fail locked keys
# Locks the password and stops there: the shell and the SSH key still work.
set -e
ssh ${KD_HOST}-worker '
printf "svc-backup ALL=(root) NOPASSWD: /usr/bin/systemctl restart kubelet\n" > /etc/sudoers.d/svc-backup
chmod 440 /etc/sudoers.d/svc-backup
usermod -L jmiller'
