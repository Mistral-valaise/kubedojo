Two processes with one name, two different treatments. First tell them apart:

```bash
ssh {{host}}
cd /opt/course/stuck-exporter
cat README
pgrep -a stuck-exporter
# 5120 /usr/bin/python3 /opt/course/stuck-exporter/bin/stuck-exporter --job nightly
# 5127 /usr/bin/python3 /opt/course/stuck-exporter/bin/stuck-exporter --job hourly
cat run/nightly.status run/hourly.status
```

**1. The hung job** (use the pid of *nightly* from your output)

```bash
kill 5120                 # SIGTERM first: always give a process the chance to stop cleanly
ps -p 5120                # still there
kill -KILL 5120           # the same as kill -9: cannot be caught or ignored
ps -p 5120                # gone
rm run/nightly.lock       # a killed process cleans up nothing
```

`kill` returning without an error only says the signal was sent. Whether the process ended is a
separate question, answered by `ps -p <pid>`.

**2. Reload the healthy job** (the pid of *hourly*)

```bash
kill -HUP 5127            # the same as kill -1
cat run/hourly.status     # pid unchanged, level=debug, reloads=1
```

| Signal | Number | Meaning | Can be ignored |
|---|---|---|---|
| `SIGHUP` | 1 | terminal closed; by convention "read your configuration again" for daemons | yes |
| `SIGINT` | 2 | what Ctrl-C sends | yes |
| `SIGTERM` | 15 | please stop; the default of `kill` | yes |
| `SIGKILL` | 9 | the kernel removes the process | no |
| `SIGSTOP` / `SIGCONT` | 19 / 18 | freeze / continue | STOP: no |

Why not `killall -9 stuck-exporter` or `pkill -9 stuck-exporter`? Both match by name and would take
the hourly job down as well. And a process that does not handle SIGHUP *dies* from it, so look at
the documentation before you send one.

**Takeaway:** SIGTERM first, check, SIGKILL only if needed and then clean up yourself; SIGHUP to
reload; and always signal a pid when a name is not unique.
