Both answers are one pipeline each. Build it step by step and look at every intermediate result.

```bash
ssh {{host}}
cd /opt/course/rank-login-shells
head -n 5 accounts.txt                   # two comment lines, then name:x:uid:gid:comment:home:shell

grep -v '^#' accounts.txt | cut -d: -f7 | sort | uniq -c | sort -rn > shells.txt
cat shells.txt
```

| Step | Does |
|---|---|
| `grep -v '^#'` | drops the comment lines (`-v` inverts the match, `^` anchors at the start of the line) |
| `cut -d: -f7` | keeps field 7 with `:` as delimiter. A line without any `:` would be printed whole |
| `sort` | brings equal shells next to each other, which `uniq` needs |
| `uniq -c` | merges neighbours and puts the count in front |
| `sort -rn` | numeric and reversed: the largest count first |

```bash
grep -v '^#' accounts.txt \
  | grep -Ev ':(/usr/sbin/nologin|/bin/false)$' \
  | sort -t: -k3,3nr | head -n 5 | cut -d: -f1 > top-uids.txt
cat top-uids.txt
```

- `grep -Ev ':(a|b)$'`: extended regular expression, lines whose last field is one of the two
  shells are dropped. Anchoring with `:` and `$` keeps a comment or home directory that merely
  contains the word from matching.
- `sort -t: -k3,3nr`: field separator `:`, the key starts and ends at field 3 (`-k3` alone would
  run to the end of the line), `n` numeric, `r` reversed. Without `n`, `999` beats `31007`.
- Filter first, then `head`, then `cut`: once the name is cut out, the UID is gone and cannot be
  sorted on any more.

`wc -l` is the quick cross-check: the counts in `shells.txt` add up to the 38 lines that
`grep -vc '^#' accounts.txt` reports.

**Takeaway:** `cut` picks the column, `sort | uniq -c | sort -rn` ranks it, and `sort -t -k N,Nn`
orders whole lines by one numeric field.
