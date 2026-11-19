Look at the state first: `ls -l` shows link targets, `-i` adds inode numbers.

```bash
ssh {{host}}
cd /opt/course/release-links/app
ls -l                                    # current -> /opt/.../releases/2024-05-02 (does not exist)
ls -li shared releases/*                 # inode and link count of every license.key
```

`current` is dangling and absolute, `previous` points to the wrong release. Of the three
`license.key` files only the one in `2024-03-01` has the inode of `shared/license.key`; the one in
`2024-04-15` is a copy (another inode, link count 1), the one in `2024-05-20` a symbolic link (`l`).

```bash
ln -sfn releases/2024-05-20 current      # -s symbolic, -f replace, -n do not follow the old link
ln -sfn releases/2024-04-15 previous
ls -l current previous

ln -f shared/license.key releases/2024-04-15/license.key    # no -s: a hard link; -f replaces
ln -f shared/license.key releases/2024-05-20/license.key
ls -li shared releases/*                 # four times the same inode, link count 4

stat -c %h shared/license.key > ../link-count.txt
```

- **Relative target.** A symbolic link stores text. `releases/2024-05-20` is resolved from the
  directory that contains the link, so it still works after `cp -a app /somewhere/else`. An absolute
  target would keep pointing into `/opt/course/release-links`.
- **`-n`.** `previous` is a link to a directory. `ln -sf releases/2024-04-15 previous` follows it
  and creates `releases/2024-03-01/2024-04-15`, a dangling link inside the old release, and
  `previous` stays what it was. `-n` says: the link itself is the destination.
- **Hard link.** `ln a b` gives the inode of `a` the second name `b`. Both names are equal; there
  is no "original". It works only within one file system and not for directories.
- **Link count.** `stat -c %h` (or the second column of `ls -l`) counts the names. `rm` removes a
  name; the data goes when the last name is gone. `find . -samefile shared/license.key` lists them.

**Takeaway:** symbolic links are paths (make them relative, replace them with `ln -sfn`); hard
links are extra names for one inode (compare with `ls -li`).
