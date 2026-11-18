`awk` splits every line at blanks into fields `$1`, `$2`, ... Count them once on a sample line:

```bash
ssh {{host}}
cd /opt/course/access-log-report
head -1 access.log | awk '{print "path=" $7, "status=" $9, "bytes=" $10}'
```

The quoted request `"GET /login HTTP/1.1"` contains blanks, so it takes three fields (6, 7 and 8);
the status is field 9 and the bytes are field 10.

**1. Server errors per path**

```bash
awk '$9 >= 500 && $9 <= 599 {print $7}' access.log | sort | uniq -c | sort -rn > errors.txt
cat errors.txt
```

| Step | What it does |
|---|---|
| `awk '$9 >= 500 && $9 <= 599 {print $7}'` | keeps the lines whose **status field** is a server error and prints their path |
| `sort` | puts equal paths next to each other: `uniq` only merges neighbours |
| `uniq -c` | one line per path, with the number of occurrences in front |
| `sort -rn` | numeric, highest first |

Why not `grep ' 500 '`? It misses 502, 503 and 504, and a pattern like `' 50[0-9] '` also matches a
successful response of 502 bytes (`200 502`). `grep 500` even matches the path `/img/500.png`. Fields
say what you mean.

**2. Bytes sent with status 200**

```bash
awk '$9 == 200 {sum += $10} END {print sum}' access.log > bytes.txt
```

`sum` starts at zero, grows on every selected line, and `END` runs once after the last line.

**Takeaway:** select by field with `awk`, then `sort | uniq -c | sort -rn` turns any list into a
ranking.
