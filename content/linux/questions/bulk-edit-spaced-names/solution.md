**1. Look before you edit**

```bash
ssh {{host}}
cd /opt/course/bulk-edit-spaced-names/sites
find . -type f -name '*.conf'
grep -rc 'db-old.internal' .        # occurrences per file, all endings
```

Seven files end in `.conf`. Four have blanks, brackets or an apostrophe in their name, one is hidden
(`blog/.cache.conf`), and `shop/checkout.conf` mentions the host twice in one line.

**2. Edit exactly these files**

```bash
find . -type f -name '*.conf' -print0 | xargs -0 sed -i 's/db-old\.internal/db-new.internal/g'
```

| Part | Why |
|---|---|
| `-type f -name '*.conf'` | selects by name: `checkout.conf.bak` and `2023 legacy.conf.disabled` do not end in `.conf` |
| `-print0` and `xargs -0` | names are separated by a NUL byte, the one character a file name cannot contain |
| `sed -i` | writes the result back into each file |
| `s/db-old\.internal/.../g` | `g` replaces every match in a line; `\.` is a literal dot (a bare `.` matches any character) |

Without `-print0`/`-0`, `xargs` splits `shop/shop frontend.conf` into two words, and aborts at the
apostrophe of `O'Brien test.conf` with "unmatched single quote". `sed` then fails for files that do
not exist, and the files with simple names are edited: a half-done change, the worst result.

Equivalent without `xargs`: `find . -type f -name '*.conf' -exec sed -i 's/.../.../g' {} +`.

**3. Verify**

```bash
grep -rl 'db-old.internal' .        # only the files with other endings are left
```

**Takeaway:** when file names come out of `find`, send them through `-print0 | xargs -0` (or
`-exec ... {} +`). Never through a plain `xargs` or `for f in $(find ...)`.
