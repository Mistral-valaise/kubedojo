ssh ${KD_HOST}-worker '
id svc-backup >/dev/null 2>&1 || useradd -r -m -s /bin/bash svc-backup
id jmiller >/dev/null 2>&1 || useradd -m -s /bin/bash jmiller
usermod -s /bin/bash jmiller
echo "jmiller:Autumn-2023-kd" | chpasswd
mkdir -p /home/jmiller/.ssh
echo "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKd0j0kubedojoExampleKeyOnlyForTheExercise000000 jmiller@laptop" > /home/jmiller/.ssh/authorized_keys
chown -R jmiller:jmiller /home/jmiller/.ssh && chmod 700 /home/jmiller/.ssh && chmod 600 /home/jmiller/.ssh/authorized_keys
echo "quarterly numbers" > /home/jmiller/report.txt && chown jmiller:jmiller /home/jmiller/report.txt
rm -f /etc/sudoers.d/*svc-backup*
printf "# backup agent\nsvc-backup ALL=(ALL) NOPASSWD: ALL\n" > /etc/sudoers.d/svc-backup
chmod 440 /etc/sudoers.d/svc-backup'
exit 0
