Look at what is there, and at who the people are:

```bash
ssh {{host}}
cd /opt/course/team-shared-dir
ls -laR project
getent group teamdir-dev            # members: teamdir-ana, teamdir-ben
id teamdir-eve                      # not in the group
```

**1. Group ownership, recursively**

```bash
sudo chgrp -R teamdir-dev project
```

`-R` also reaches hidden files and subdirectories, which `project/*` does not.

**2. Rights for the group, none for others**

```bash
sudo chmod -R u+rwX,g+rwX,o-rwx project
```

The capital `X` means "execute, but only for directories and for files that are executable
already". So directories become `rwxrwx---`, text files `rw-rw----`, and `src/build.sh` stays a
program. A plain `chmod -R 770` would make every file executable.

**3. New entries inherit the group: setgid on every directory**

```bash
sudo find project -type d -exec chmod g+s {} +
```

Without setgid a file created by `teamdir-ben` belongs to his primary group `teamdir-ben`, and the
team is locked out again. With it, new files get the group of the directory, and new directories
get the bit as well.

**4. Sticky bit on inbox**

```bash
sudo chmod +t project/inbox
ls -ld project project/inbox
# drwxrws---  root teamdir-dev  project           s = setgid (octal 2770)
# drwxrws--T  root teamdir-dev  project/inbox     T = sticky  (octal 3770)
```

**Test it as the people concerned**

```bash
sudo -u teamdir-ben touch project/docs/test && sudo ls -l project/docs/test   # group teamdir-dev
sudo -u teamdir-ben rm project/inbox/from-ana.txt     # Operation not permitted
sudo -u teamdir-eve ls project                        # Permission denied
sudo -u teamdir-ben rm project/docs/test
```

One thing setgid does not solve: the *mode* of a new file comes from the creator's `umask`. With the
usual `022` a new file is `rw-r--r--`, readable but not writable for the team. Teams solve that with
`umask 002` or with a default ACL on the directories (`setfacl -d -m g::rwX project`).

**Takeaway:** a team directory is `chgrp -R` plus mode `2770` on directories (`3770` where files need
protection from colleagues), `660` on files, and never `777`.
