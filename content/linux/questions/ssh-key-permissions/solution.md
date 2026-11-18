Look first, then change as little as needed.

```bash
ssh {{host}}
cd /opt/course/ssh-key-permissions/deploy
ls -la .ssh
ssh-keygen -y -f .ssh/id_ed25519        # "UNPROTECTED PRIVATE KEY FILE": mode 0644 is too open
```

**1. The modes ssh expects**

```bash
chmod 700 .ssh                          # drwx------  only the owner enters, lists, changes
chmod 600 .ssh/id_ed25519 .ssh/config   # -rw-------  only the owner reads and writes
ssh-keygen -y -f .ssh/id_ed25519        # now prints the public key
```

| Octal | Symbolic | Meaning |
|---|---|---|
| `700` | `u=rwx,go=` | a private directory: `x` is what lets you enter it, so `600` would lock you out as well |
| `600` | `u=rw,go=` | a private file |
| `644` | `u=rw,go=r` | a file everybody may read, such as `id_ed25519.pub` |

Each digit is the sum of read 4, write 2 and execute 1, for user, group and others. The public key
may stay readable: it is public.

**2. One more reader**

The mode bits know one owner, one group and "everybody else". `chmod 644` would serve the auditor and
everybody else with it. An ACL adds an entry for exactly one user:

```bash
setfacl -m u:sshkeyperm-audit:r release.env
getfacl release.env                     # user:sshkeyperm-audit:r--, group::---, other::---
ls -l release.env                       # -rw-r-----+   the + says: there is an ACL
sudo -u sshkeyperm-audit cat release.env    # test as that account
sudo -u nobody cat release.env              # Permission denied, as it should be
```

With an ACL, the group column of `ls -l` shows the ACL *mask* (the upper limit for all named entries),
not the rights of the group: trust `getfacl`. `setfacl -x u:sshkeyperm-audit release.env` removes the
entry again, `setfacl -b` removes the whole ACL.

**Takeaway:** `700` for the directory, `600` for the key, and when exactly one more account needs
access, add an ACL entry instead of opening the file to everybody.
