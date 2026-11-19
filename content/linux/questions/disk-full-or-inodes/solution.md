"No space left on device" has two possible causes, and `df -h` shows only one of them.

**1. Read the symptom.**

```bash
ssh {{host}}
cd /opt/course/disk-full-or-inodes
cat incident.txt
```

`df -h`: 68% used, 6.2G free. `df -i`: `IUse%` 100, `IFree` 0. The file system has space but no
free **inode**, and every new file needs one: `exhausted: inodes`. On a real host you would run
`df -h` and `df -i` yourself; here the output was recorded for you.

**2. Find what occupies the space.**

```bash
ls -lhS volume/db volume/uploads/tmp       # pages.img: 1.0G ... the biggest?
du -a volume | sort -rn | head -n 5        # no: core.20391 with about 24 MiB
ls -lsh volume/db/pages.img                # first column: blocks really used (0)
```

`ls -l` shows the length of a file. `pages.img` is **sparse**: a gigabyte long with nothing written,
so it occupies no blocks. `du` (and `ls -s`, `find -printf '%b'`) count blocks, and that is what a
full disk is about.

**3. Find where the files are.**

```bash
find volume -type f -printf '%h\n' | sort | uniq -c | sort -rn | head -n 3
#   2700 volume/cache/sessions
#     50 volume/mail/queue/9
du -sh volume/cache/sessions               # tiny: empty files use inodes, not space
```

`%h` is the directory a file is in; `sort | uniq -c | sort -rn` is the counting idiom.

```bash
b=/opt/course/disk-full-or-inodes
printf '%s\n' 'exhausted: inodes' \
  "largest-file: $b/volume/uploads/tmp/core.20391" \
  "most-files: $b/volume/cache/sessions" > report.txt
```

**4. Clean up exactly that.**

```bash
rm volume/uploads/tmp/core.20391
find volume/cache/sessions -type f -mtime +7 | wc -l      # look first: 2400
find volume/cache/sessions -type f -mtime +7 -delete
find volume/cache/sessions -type f | wc -l                # 300 live sessions remain
du -sh volume
```

Always run the `find` without `-delete` first, and put `-delete` last: `find` evaluates from left to
right, so a `-delete` in front of the tests deletes everything. `rm volume/cache/sessions/*` would
log out every customer, and with enough files it fails with "Argument list too long".

**Takeaway:** `df -h` and `df -i` together; `du` for space, a count per directory for inodes; delete
by a test you have looked at first, never the whole directory.
