"Permission denied" has three possible causes along a path: a directory you cannot enter, a file you
cannot read, or a program you cannot execute. Here all three are present. Work from the account to
the file.

```bash
ssh {{host}}
cd /opt/course/unreadable-config
sudo -u unreadable-config-svc app/run-report.sh       # reproduce: Permission denied
id unreadable-config-svc                              # groups: unreadable-config-svc, unreadable-config-ops
sudo namei -l /opt/course/unreadable-config/app/conf/report.conf
sudo ls -l app
```

`namei -l` prints one line per step of the path (shortened):

```
drwxr-x--- root unreadable-config-ops app            ok: the group may enter
drwx------ root root                  conf           wrong group, and only the owner gets in
-rw------- root unreadable-config-ops report.conf    right group, but the group has no rights
```

For each line ask: is the account the owner? If not, is it in the group? If not, "others" apply. The
kernel uses exactly **one** of the three sets, the first that matches.

```bash
sudo chgrp unreadable-config-ops app/conf     # 1. the directory gets the operators' group ...
sudo chmod 750 app/conf                       #    ... which may list and enter it (r-x), not write
sudo chmod 640 app/conf/report.conf           # 2. the group may read the file
sudo chmod 750 app/run-report.sh              # 3. a script needs r and x: bash has to read it
sudo -u unreadable-config-svc app/run-report.sh
# report ok: region=eu-central recipients=3
sudo -u nobody cat app/conf/report.conf       # still: Permission denied
```

What not to do:

- `chmod 755 conf; chmod 644 report.conf`: the job runs, and every account can read the password.
- `chown unreadable-config-svc ...`: an owner can always `chmod` its own files, so the job could
  rewrite its configuration.
- Adding yourself or the account to more groups: the membership was already right. Group changes also
  reach running processes only after a new login.

ACL entries are an equally tight alternative when you may not touch owner and group:
`sudo setfacl -m u:unreadable-config-svc:x app/conf` and `... :r app/conf/report.conf`.

**Takeaway:** reproduce as the account (`sudo -u`), read the whole path with `namei -l`, compare it
with `id`, and grant the smallest right to the group that is meant to have it.
