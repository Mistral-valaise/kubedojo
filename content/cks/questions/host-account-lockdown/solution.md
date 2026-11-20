```bash
ssh {{worker}}
sudo -i
```

**1. svc-backup**

```bash
sudo -l -U svc-backup            # (ALL) NOPASSWD: ALL
visudo -f /etc/sudoers.d/svc-backup
```

```
svc-backup ALL=(root) NOPASSWD: /usr/bin/systemctl restart kubelet
```

```bash
visudo -c                        # parsed OK
sudo -l -U svc-backup            # (root) NOPASSWD: /usr/bin/systemctl restart kubelet
```

The command is listed with its arguments. `/usr/bin/systemctl` alone would allow `systemctl edit`,
`systemctl stop`, a pager with a shell escape — everything.

**2. jmiller**

```bash
usermod -L jmiller                       # lock the password
usermod -s /usr/sbin/nologin jmiller     # no interactive login
rm /home/jmiller/.ssh/authorized_keys    # keys ignore the password lock
passwd -S jmiller                        # jmiller L ...
getent passwd jmiller
```

The account stays, so `ls -l` keeps showing a name instead of a bare UID and the files can be handed
over. Deleting it (`userdel -r`) is a later step, after the data was reviewed.
