A file inside an archive cannot be edited, and the live file must not be. So the bundle is built in
three steps: copy, redact the copy, pack the copy.

**1. Look at what is there, hidden files and modes included.**

```bash
ssh {{host}}
cd /opt/course/support-bundle
find app -type f -printf '%m %8s %p\n' | sort -k3
grep -rnI -i -e password -e key app/etc          # where do secrets live?
```

`app/etc/.features` is easy to miss, `server.key` and `db.env` are the files to leave out, and
`app.conf` has a `db_password` line.

**2. Copy the requested files to a scratch directory, with their modes.**

```bash
stage=$(mktemp -d)
tar -cf - --exclude='*.key' --exclude='*.env' \
    app/etc app/log/app.log app/log/app.log.1 app/bin/run.sh | tar -xpf - -C "$stage"
```

The first `tar` writes an archive to standard output (`-f -`), the second unpacks it below `$stage`
(`-C`) with permissions (`-p`). Quote the patterns, or the shell expands them. Because the directory
`app/etc` is named, and not `app/etc/*`, its hidden file comes along. `rsync -a --exclude ...` or
`cp -a` do the same job; a plain `cp -r` would create the copies with modes cut down by the umask.

**3. Redact in the copy.**

```bash
sed -i 's/^db_password *=.*/db_password = REDACTED/' "$stage/app/etc/app.conf"
diff app/etc/app.conf "$stage/app/etc/app.conf"   # exactly one line differs
```

In Vim: `/db_password`, then `f=` `w` `C` `REDACTED` `<Esc>` `:wq`.

**4. Pack, with paths relative to the parent directory.**

```bash
tar -czf bundle.tar.gz -C "$stage" app
rm -rf "$stage"
```

`-C dir` makes tar change into `dir` first, so the members are called `app/...` and not
`tmp/tmp.Xy12/app/...`.

**5. Check the bundle like its receiver would.**

```bash
tar -tzvf bundle.tar.gz                           # 7 files, modes in the first column
tar -xzOf bundle.tar.gz | grep -a -i -e password -e 'PRIVATE KEY' -e API_KEY
# db_password = REDACTED           <- the only hit
ls -l app/etc app/log                             # the originals: untouched
```

`-O` unpacks to standard output: a quick way to search everything an archive contains.

**Takeaway:** stage, redact, pack, then list and grep the result. Exclude by pattern, include by
directory, and never touch the originals.
