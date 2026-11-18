Get to know the file, then answer each question with a pipeline that selects by **position**.

```bash
ssh {{host}}
cd /opt/course/error-log-triage
wc -l app.log                            # 420 lines
head -n 5 app.log                        # the format: date time [service] LEVEL message
grep -c ERROR app.log                    # 59 ...
grep ERROR app.log | grep -v '\] ERROR ' | head -n 3     # ... but some are INFO lines about errors
```

**1. Lines per level.** The level is field 4 when the line is split at spaces:

```bash
cut -d' ' -f4 app.log | sort | uniq -c | sort -rn > levels.txt
```

**2. Errors per service.** Select the ERROR lines by where the level stands, right after the closing
bracket of the service; then cut out the service, strip the brackets, count and rank:

```bash
grep '\] ERROR ' app.log | cut -d' ' -f3 | tr -d '[]' | sort | uniq -c | sort -rn > errors-by-service.txt
```

**3. First and last error of payments.** `-F` takes the pattern as fixed text, so the brackets need
no escaping, and `[payments]` cannot match `[payments-worker]`:

```bash
grep -F '[payments] ERROR ' app.log | head -n 1 >  payments-window.txt
grep -F '[payments] ERROR ' app.log | tail -n 1 >> payments-window.txt
grep -n -F '[payments] ERROR ' app.log   # -n: where in the file they are
```

| Tempting | What it really selects |
|---|---|
| `grep ERROR` | every line with the word: 21 INFO lines of `search` and 2 of `payments` included |
| `grep payments` | `payments` and `payments-worker` |
| `uniq -c` without `sort` in front | counts runs of equal neighbours, not totals |

To read around a hit, `grep -n -B2 -A2 -F '[payments] ERROR ' app.log` prints two lines of context
before and after each match; in `less app.log`, `/\] ERROR ` searches and `n` / `N` step through.

**Takeaway:** in structured logs match the field, not the word, and prove the pattern on a few
lines (`head`, `grep -c`, `wc -l`) before you trust its count.
