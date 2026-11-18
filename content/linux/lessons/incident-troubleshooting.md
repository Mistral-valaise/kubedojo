# Incident Troubleshooting

**8% of the skills assessment.** No new commands here: the tasks combine what you already know,
under time pressure. What counts is the order of your steps, and that you do not make it worse.

## The 20% that solves 80%

### A fixed order: look, narrow down, change one thing, verify

```bash
df -h; df -i                             # full disk? out of inodes?
free -m; uptime                          # memory, load
ps aux --sort=-%mem | head               # what runs, what is big
ss -ltn                                  # what listens
tail -n 50 /var/log/app/app.log          # what does the service itself say?
```

Change one thing at a time and test after each change. A service stops at the first error it
meets, so the second problem only becomes visible once the first is fixed: start, read the log,
fix, start again.

### "No space left on device": two budgets

```bash
df -h /srv/data                          # blocks: Use%
df -i /srv/data                          # inodes: IUse%; 100% = no new file, whatever df -h says
du -xh --max-depth=1 /srv/data | sort -h # which directory occupies the space
du -a /srv/data | sort -rn | head        # ... down to single files
find /srv/data -xdev -type f -printf '%h\n' | sort | uniq -c | sort -rn | head   # files per directory
```

- `ls -l` shows the **length** of a file, `du` and `ls -s` what it **occupies**. A sparse file
  (database image, VM disk) can be gigabytes long and use nothing: deleting it frees nothing.
- Thousands of empty files use no space and one inode each: `du` does not see them, the count does.
- A deleted file that a process still holds open keeps its space until the process ends
  (`lsof +L1` lists them): restart the process or truncate with `: > file`.

Clean up by a test, and look before you delete:

```bash
find /srv/data/cache -type f -mtime +7 | wc -l      # first: what would go?
find /srv/data/cache -type f -mtime +7 -delete      # then delete; -delete comes LAST
```

`-mtime +7` is "more than 7 days", `-mtime 7` exactly 7. `rm dir/*` skips hidden files and fails
with "Argument list too long" when there are too many; `find` has neither problem.

### A service that fails after a change: log, diff, permissions

```bash
tail -n 20 var/log/app.log               # 1. the first ERROR after the last "starting"
diff app.conf.bak app.conf               # 2. what did the change really change?
ls -l etc/secret.token; id appuser       # 3. may the service account do what it needs?
sudo -u appuser head -c 1 etc/secret.token          # test as the user the service runs as
```

- Fix the line that is wrong, not the whole file: restoring the backup also undoes what the
  change was made for.
- "Permission denied" for a service: give **that account** the access (`chown`, or `chgrp` with
  mode 640). `chmod 777`, `644` on a secret, or running the service as root hide the problem
  and create a worse one.
- A missing directory in an error message is usually a typo in the path, not a directory to create.

### One request, several logs

```bash
grep ORD-7731 gateway.log                # from what the customer knows to the request id
grep -hw 7f3a9c *.log                    # every line of that request; -h no file names, -w whole word
grep -hw 7f3a9c *.log | sort             # in time order, if all lines start with the same timestamp format
jq -c 'select(.request_id == "7f3a9c")' orders.log  # JSON logs: select by field
```

Read along the call chain. "upstream failed" and "service X returned 504" are messengers; the
origin is the deepest service that reports an error **of its own**. Without `-w`, a short id also
matches inside longer strings and foreign lines slip into the trace.

### Collecting evidence: a support bundle

```bash
stage=$(mktemp -d)
tar -cf - --exclude='*.key' --exclude='*.env' app/etc app/log/app.log | tar -xpf - -C "$stage"
sed -i 's/^db_password *=.*/db_password = REDACTED/' "$stage/app/etc/app.conf"     # redact the COPY
tar -czf bundle.tar.gz -C "$stage" app   # -C: members are called app/..., not /tmp/...
tar -tzvf bundle.tar.gz                  # list: paths, modes
tar -xzOf bundle.tar.gz | grep -a -i -e password -e 'PRIVATE KEY'    # search everything inside
```

Name the directory (`app/etc`), not `app/etc/*`: the pattern skips hidden files. Copy with
`cp -a`, `rsync -a` or a tar pipe; plain `cp -r` loses modes and times.

## Habits that save points

- Evidence first: note `wc -l` of a log before you start, copy before you edit
  (`cp -p app.conf app.conf.$(date +%s)`), filter logs into a new file, never in place.
- Never delete what you have not listed. Run the `find` without `-delete`, the `rm` as `ls` first.
- The quick fix is graded too: `rm -rf` of the directory, `chmod 777`, an emptied log or a service
  started as root make the symptom disappear and fail the task.
- Verify like the user: the health URL answers, the file can be created, the bundle unpacks. A
  command that printed no error has proven nothing yet.
- Write down cause and fix in one sentence each. If you cannot, you have not found the cause.
