`find` walks a directory tree and prints every entry for which **all** of its tests hold.

```bash
ssh {{host}}
find /opt/course/find-large-logs/logs -type f -name '*.log' -size +100k -mtime +30 \
  | sort > /opt/course/find-large-logs/result.txt
cat /opt/course/find-large-logs/result.txt
```

| Test | Meaning | The trap |
|---|---|---|
| `-type f` | regular files only | without it, the directory `old.log` and the symbolic link `web/latest.log` are listed |
| `-name '*.log'` | the name ends in `.log` | quote the pattern, or the shell expands it before `find` sees it; `server.log.1` does not match |
| `-size +100k` | larger than 100 KiB | `+100` without a unit counts 512-byte blocks; `access.log` is exactly 100 KiB and therefore not larger |
| `-mtime +30` | modified more than 30 days ago | `-mtime 30` means exactly 30 days, `-mtime -30` means newer |

`find` prints in directory order, so `sort` comes last. One path contains a space (`db/slow query.log`):
writing the output of `find` straight to the file keeps it intact, while a loop like
`for f in $(find ...)` would split it in two.

To check a result by eye, let `find` show size and date as well:

```bash
find /opt/course/find-large-logs/logs -name '*.log' -printf '%y %10s %TY-%Tm-%Td %p\n' | sort -k4
```
