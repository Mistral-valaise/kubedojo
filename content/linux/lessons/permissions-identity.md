# Permissions & Identity

**8% of the skills assessment.** Every "Permission denied" has the same three questions behind it:
who am I, who owns the thing, and which of the nine mode bits applies to me?

## The 20% that solves 80%

### Who am I, who is it

```bash
id                          # uid, primary group, all groups of the current user
id deploy; groups deploy    # the same for another account
ls -l file; ls -ld dir      # mode, owner, group     (-d: the directory itself, not its content)
stat -c '%a %U:%G %n' file  # mode as a number: 640 root:ops file
namei -l /srv/app/conf/app.conf      # owner, group and mode of EVERY step of a path
sudo -u deploy cat /srv/app/conf/app.conf    # try it as the account that has the problem
```

### Reading a mode

```
-rw-r-----  1 root ops  report.conf        user: rw-   group: r--   others: ---   = 640
drwxr-x---  2 root ops  conf               user: rwx   group: r-x   others: ---   = 750
```

- `r` = 4, `w` = 2, `x` = 1; one digit each for **u**ser (owner), **g**roup, **o**thers.
- The kernel uses exactly **one** set: owner if you are the owner, else group if you are in the
  group, else others. A member of the group with `---` for the group is denied even when others may read.
- **Directories:** `r` list the names, `x` enter and reach what is inside, `w` create, rename and
  **delete** entries. Deleting a file needs `w` on the directory, not on the file.
- To open a file you need `x` on **every** directory above it. A script needs `r` and `x`.

### chmod, chown, chgrp

```bash
chmod 640 file               # set all nine bits at once
chmod u=rw,g=r,o= file       # the same, symbolic
chmod g+w,o-rwx file         # change only what you name
chmod -R g+rwX,o-rwx dir     # X: execute for directories and for files that are executable already
chgrp -R ops dir             # change the group       (needs sudo unless you own it and are in ops)
sudo chown deploy:ops file   # owner and group        (changing the owner always needs root)
find dir -type d -exec chmod 2770 {} +     # directories and files separately
find dir -type f -exec chmod 660 {} +
```

Modes worth knowing by heart: `600` private file, `644` public file, `700` private directory,
`755` public directory or program, `640`/`750` "owner and one group".

### Shared directories: setgid and sticky

```bash
chmod g+s dir     # 2xxx  drwxrws---   new entries inherit the GROUP of the directory
chmod +t dir      # 1xxx  drwxrwx--T   only the owner of a file may delete or rename it (like /tmp)
chmod 2770 dir    # team directory        chmod 3770 dir    # team directory with protected files
```

Without setgid a new file belongs to the creator's primary group, and the rest of the team is
locked out again.

### umask: the mode of new files

```bash
umask             # 0022: new files 644, new directories 755
umask 027         # for this shell: files 640, directories 750
```

The umask takes bits **away** from 666 (files) and 777 (directories). It belongs to the process:
setting it in your shell changes nothing for other users or services.

### One more user: ACLs

```bash
setfacl -m u:audit:r file      # one extra entry: user audit may read
getfacl file                   # the whole truth; ls -l only shows a + after the mode
setfacl -x u:audit file        # remove the entry        setfacl -b file   # remove the ACL
setfacl -d -m g::rwX dir       # default ACL: what new entries in dir start with
```

With an ACL, the group column of `ls -l` shows the *mask* (upper limit of all named entries), not
the rights of the group.

### sudo

```bash
sudo -l                        # what may I run?
sudo -u deploy <command>       # as another user, not as root
sudo vim /etc/app.conf         # edit a file of root
sudo sh -c 'echo x >> /etc/f'  # sudo echo x >> /etc/f fails: the redirection is done by YOUR shell
echo x | sudo tee -a /etc/f    # the usual alternative
```

### ssh is strict about modes

`~/.ssh` must be `700`, a private key `600` (or `400`), `authorized_keys` and `config` not writable
by anybody else. ssh silently ignores a key that is too open: `ssh -v` and `ssh-keygen -y -f <key>`
say so.

## Habits that save points

- Never `chmod 777`, and never `chmod -R` with a plain number on a tree: it makes every file
  executable. Use `X`, or `find -type d` / `-type f`.
- Fix the cause on the path, not only the file the error names: `namei -l` shows all of it.
- Give access to the smallest set: the existing group before "others", an ACL entry before a new
  group, read before write.
- Test as the user concerned (`sudo -u <user> ...`), and test that an outsider is still denied
  (`sudo -u nobody cat file`).
- New group memberships apply to new logins only: `id` in the running shell still shows the old list.
- `-rwxr-xr-x` on a file you cannot run? Look at the directories above it, and at `mount` options.
