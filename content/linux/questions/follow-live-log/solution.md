A file that is still being written needs a tool that keeps reading: `tail -f`.

```bash
ssh {{host}}
cd /opt/course/follow-live-log
tail -n 5 log/service.log                # the last lines, once: what does the log look like?
grep 'reload#' log/service.log | tail -n 4       # older reloads: the last one is reload#3

bin/request-reload; tail -f log/service.log | grep --line-buffered 'reload#'
```

`tail -f` prints every line the service adds, and `grep` lets only the reload lines through
(`--line-buffered` makes it pass each match on at once; it matters as soon as the output of `grep`
goes into another pipe or a file instead of the terminal). Within a few
seconds four lines appear, the last one `reload#4 finished token=...`. Press Ctrl-C: it ends
`tail` and `grep`, not the service.

Now the number is known, and the file has kept everything:

```bash
grep -F ' reload#4 ' log/service.log > reload.log          # your number instead of 4
cat reload.log
grep -o 'token=.*' reload.log | cut -d= -f2 > token.txt
cat token.txt
```

- `-F ' reload#4 '` is fixed text with the spaces around it: it matches neither `reload#40` nor
  the scheduler's "next reload check" lines nor reload#1 to #3.
- `grep -o` prints only the part of the line that matched; `cut -d= -f2` keeps what follows `=`.

Other ways to watch a growing file:

| Command | Use |
|---|---|
| `tail -f file` | follow; `tail -n 0 -f` starts with no old lines, `tail -n +120 -f` starts at line 120 |
| `tail -F file` | follow by name: keeps working when the file is rotated and created anew |
| `less +F file` | follow inside `less`; Ctrl-C stops following so you can scroll and search, `F` resumes |

A script cannot press Ctrl-C. It notes the length of the log, triggers, and lets the reader end the
pipeline at the line it waited for:

```bash
before=$(wc -l < log/service.log)
bin/request-reload
tail -n "+$((before + 1))" -f log/service.log | sed -n '/ reload#[0-9]* /p; / reload#[0-9]* finished /q' > reload.log
```

**Takeaway:** `tail -f` (or `less +F`) to see events as they happen, `grep` on the file afterwards
to collect them, and always search for your own marker, not for a word that others log too.
