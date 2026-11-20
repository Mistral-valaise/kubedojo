set -e
ssh ${KD_HOST}-worker '
set -e
printf "# backup agent: may restart the kubelet, nothing else\nsvc-backup ALL=(root) NOPASSWD: /usr/bin/systemctl restart kubelet\n" > /etc/sudoers.d/svc-backup
chmod 440 /etc/sudoers.d/svc-backup
visudo -c >/dev/null
usermod -L jmiller
usermod -s /usr/sbin/nologin jmiller
rm -f /home/jmiller/.ssh/authorized_keys'
