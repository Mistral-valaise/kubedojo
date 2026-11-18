Three questions, three ways to use `grep` on a tree: files **with** a match (`-l`), a **count** of
matching lines, files **without** a match (`-L`). Look at the matching lines first, with `-n`, and
switch to `-l` or a count when the pattern is right.

```bash
ssh {{host}}
b=/opt/course/grep-config-tree

grep -rnE --include='*.conf' 'ssl_protocols' $b/etc           # look: comments, TLSv1.2, a tab
grep -rlE --include='*.conf' '^[[:space:]]*ssl_protocols.*TLSv1(\.1)?[ ;]' $b/etc > $b/weak-tls.txt
```

- `-r` walks the tree, `--include='*.conf'` skips `old-shop.conf.bak`, `-l` prints file names only.
- `^[[:space:]]*ssl_protocols`: from the start of the line only blanks or tabs may come before the
  directive, so `# ssl_protocols ...` does not match.
- `TLSv1(\.1)?[ ;]`: `TLSv1`, optionally `.1`, then a space or the semicolon. `TLSv1.2` fails at
  the `.2`. (`-w` would not help: for grep a word ends at the dot.)

```bash
grep -rnE --include='*.conf' '10.20' $b/etc                   # look: 110.20.1.5, 1012034, comments
grep -rhE --include='*.conf' '^[^#]*(^|[^0-9.])10\.20\.[0-9]+\.[0-9]+' $b/etc | wc -l > $b/decom-count.txt
```

- `\.` is a literal dot; a bare `.` matches any character and finds `serial = 1012034`.
- `(^|[^0-9.])` demands the start of the line or a character that is neither digit nor dot in
  front, which rejects `110.20.1.5`. `10\.20\.` rejects `10.200.0.0`.
- `^[^#]*` allows no `#` before the address: comment lines drop out.
- `-h` omits the file names. `grep -c` would print one count per file, not the total.

```bash
grep -LE '^[[:space:]]*server_name' $b/etc/nginx/sites/*.conf > $b/no-server-name.txt
```

`-L` lists the files with no matching line. The shell glob restricts the search to `*.conf`
directly in `sites`, so `-r` is not needed.

When one pattern gets too clever, split it: `grep -v '^[[:space:]]*#' file | grep ...` first drops
comments, then searches. Across a tree that needs `-r` with file names (`grep -rv ... | grep ...`),
so the single anchored pattern is usually simpler.

**Takeaway:** `grep -rn` to look, then `-l`, `-L` or `-h | wc -l` for the answer; anchor with `^`,
escape the dots, and say what may stand before and after the thing you search.
