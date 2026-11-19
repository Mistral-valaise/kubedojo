**1. See the noise, then remove it**

```bash
ssh {{host}}
cd /opt/course/config-drift
diff -u web-01/app.conf web-02/app.conf | less
```

`diff -u` prints lines of the first file with `-` and lines of the second with `+`. Here that is
most of the file: spacing, comments, headings. Bring both files into one normal form and compare
those instead:

```bash
norm() { grep -v '^#' "$1" | tr -d ' \t' | grep -v '^$' | sort; }
diff <(norm web-01/app.conf) <(norm web-02/app.conf)
```

| Step | Removes |
|---|---|
| `grep -v '^#'` | comment lines |
| `tr -d ' \t'` | every blank and tab, so `a = b`, `a=b` and `a<TAB>=<TAB>b` become equal |
| `grep -v '^$'` | blank lines |
| `sort` | differences in order |

`<(command)` hands the output of a command to `diff` as if it were a file. What is left:

```
< hostname=web-01                 expected
< listen_address=10.20.0.11       expected
< rate_limit_per_minute=120       missing on web-02
< tls_min_version=1.2             other value ...
> debug_endpoints=on              only on web-02
> hostname=web-02
> listen_address=10.20.0.12
> tls_min_version=1.0             ... on web-02
```

(`tr -d ' '` also squeezes the value `Welcome to the shop`; it does so on both sides, so the
comparison still holds. `diff -uwB` is the quick alternative: `-w` ignores blanks, `-B` blank lines.
It still shows the comments.)

```bash
printf '%s\n' debug_endpoints rate_limit_per_minute tls_min_version > drift.txt
```

**2. Repair web-02 only where it is wrong**

In Vim: `vim web-02/app.conf`, `/tls_min` to jump, `cw` on the value; `/debug_end` and `dd`;
`/rate_limit_burst`, `O` and type the missing line. Or with `sed -i`:

```bash
sed -i -e 's/^tls_min_version.*/tls_min_version = 1.2/' \
       -e '/^debug_endpoints/d' \
       -e '/^rate_limit_burst/i rate_limit_per_minute = 120' web-02/app.conf
diff <(norm web-01/app.conf) <(norm web-02/app.conf)      # only the two host settings remain
```

`s/old/new/` substitutes, `/pattern/d` deletes the matching line, `/pattern/i text` inserts a line
before it. `cp web-01/app.conf web-02/app.conf` would also remove the drift, and with it the
identity of web-02.

**Takeaway:** normalise both sides, then diff. `diff <(...) <(...)` compares what matters instead of
what is in the file.
