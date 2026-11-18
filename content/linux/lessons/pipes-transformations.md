# Pipes & Transformations

**12% of the skills assessment.** Small tools that each do one thing, connected so that the output of
one is the input of the next. Build a pipeline one stage at a time and look at the output after each.

## The 20% that solves 80%

### Streams and redirection

Every command has one input and two outputs: stdin (0), stdout (1) for results, stderr (2) for complaints.

```bash
cmd > out.txt                 # stdout to a file (replaces it); >> appends
cmd > out.txt 2> err.txt      # stderr has its own operator: each stream to its own file
cmd > all.txt 2>&1            # both into one file: "2 goes where 1 goes now", so the order matters
cmd 2>/dev/null               # throw the complaints away
cmd < in.txt                  # the file becomes stdin
cmd1 | cmd2                   # stdout of cmd1 becomes stdin of cmd2; stderr still goes to the terminal
cmd | tee out.txt             # see the output and save it (tee -a appends)
echo "$?"                     # exit code of the last command: 0 = success. Read it at once
```

- After a pipeline, `$?` belongs to the **last** command. All codes: `"${PIPESTATUS[@]}"`.
- `sort file > file` empties the file before `sort` reads it. Write to another file, then `mv`.
- `sudo echo x > /etc/file` fails: the redirection is done by your shell. Use `echo x | sudo tee /etc/file`.

A here-document feeds lines to stdin; quote the delimiter to keep `$` literal:

```bash
cat > app.conf <<'EOF'
path = $HOME/app
EOF
```

### Count and rank: sort, uniq, cut, tr, wc

```bash
sort -n file                  # numeric; -r reverse; -u unique; -k2,2 by field 2; -t: sets the separator
sort | uniq -c | sort -rn     # THE ranking idiom: uniq only merges neighbours, so sort first
cut -d: -f1,7 /etc/passwd     # fields 1 and 7, separator ":" (exactly one character)
tr -s ' ' | cut -d' ' -f3     # squeeze runs of blanks first, or cut counts empty fields
tr 'a-z' 'A-Z'                # translate characters; tr -d '\r' deletes them
wc -l                         # count lines; head -n 5 keeps the first five
```

### awk: fields, conditions, sums

awk splits each line at blanks into `$1`, `$2`, ... (`$0` is the line, `NF` the number of fields).

```bash
awk '{print $1, $7}' access.log                       # columns
awk '$9 >= 500 {print $7}' access.log                 # condition on a FIELD, not on the whole line
awk -F: '$3 >= 1000 {print $1}' /etc/passwd           # another separator
awk '$9 == 200 {sum += $10} END {print sum}' access.log        # add up a column
awk '{count[$1]++} END {for (k in count) print count[k], k}'   # count per key
```

Count the fields on one sample line first: `head -1 file | awk '{print $7, $9}'`. Text that only
looks like the field (`grep 500`) also matches paths, sizes and timestamps.

### sed: substitute, delete, insert

```bash
sed 's/old/new/' file              # first match in each line, result to stdout; s/old/new/g every match
sed -n '10,20p' file               # print lines 10 to 20 only
sed '/^#/d' file                   # delete lines that match
sed '/^listen/s/80/8080/' file     # substitute only on lines that match the address
sed '/^port/i # added' file        # insert a line before the match (a appends after it)
sed -i 's|/old/path|/new/path|g' file   # edit the file in place; any delimiter works
```

`.` matches any character: write `\.` for a dot. Run without `-i` first, or keep a copy with `-i.bak`.

### Many files: find, xargs, command substitution

```bash
find . -name '*.conf' -print0 | xargs -0 sed -i 's/old/new/g'   # safe for blanks and quotes in names
find . -name '*.conf' -exec grep -l old {} +                    # the same without xargs
today=$(date +%F); tar czf "backup-$today.tgz" conf/            # $(...) inserts a command's output
```

Plain `find | xargs` and `for f in $(find ...)` split names at blanks. Always `-print0 | xargs -0`.

### Compare: diff

```bash
diff -u old.conf new.conf          # - lines only in the first file, + lines only in the second
diff -uwB a b                      # ignore blanks (-w) and blank lines (-B)
diff <(sort a) <(sort b)           # compare the OUTPUT of two commands: normalise, then diff
```

### JSON: jq

```bash
jq . file.json                                  # pretty-print; jq '.items | length' counts
jq -r '.items[].metadata.name' pods.json        # -r: raw strings, ready for sort, uniq, wc
jq -r '.items[] | select(.status.phase != "Running") | "\(.metadata.namespace)/\(.metadata.name)"' pods.json
jq '[.items[].status.containerStatuses[]?.restartCount] | add' pods.json   # collect numbers, add them
```

`.a[]` walks an array, `select()` filters, `"\(...)"` builds text, `[]?` and `// 0` survive missing
fields. With a cluster: `kubectl get pods -A -o json | jq -r '...'`.

## Habits that save points

- Build left to right: run the first stage, look, add `| next`, look again. Add `| head` while testing.
- Read the required output format twice: order, separator, one item per line. Then `cat` your file.
- Select by field or by file name first, then transform. Never edit what you have not listed.
- Results go to a new file. The input stays untouched until you have looked at the result.
