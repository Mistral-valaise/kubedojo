# Text Search & Logs

**10% of the skills assessment.** Find the lines that matter in files that are too long to read:
look at a sample, write a pattern that matches the field and not just the word, then count.

## The 20% that solves 80%

### Look at a file

```bash
wc -l app.log                 # how many lines? decides whether to read or to search
head -n 5 app.log             # the first lines: the format
tail -n 20 app.log            # the last lines: the newest entries
tail -n +2 data.csv           # from line 2 on: skip a header
sed -n '120,130p' app.log     # lines 120 to 130
less app.log                  # page through it: /pattern search, n/N next/previous, G end, g start, q quit
less -S -N app.log            # do not wrap long lines; show line numbers
cat -A file                   # make tabs (^I) and line ends ($) visible
```

Use `cat` for short files and for feeding pipelines, `less` for everything else: it does not flood
the terminal, and `&pattern` inside it shows only the matching lines.

### grep: select lines

```bash
grep 'timeout' app.log                  # lines that contain the text
grep -i 'timeout' app.log               # ignore case
grep -v '^#' app.conf                   # invert: lines that do NOT match
grep -n 'timeout' app.log               # with line numbers
grep -c 'timeout' app.log               # count matching lines (per file)
grep -F '[payments] ERROR' app.log      # fixed text: no character is special
grep -w 'error' app.log                 # whole word; note that a dot or dash ends a word
grep -o 'token=[0-9a-f]*' app.log       # print only the matching part
grep -A 3 -B 1 'panic' app.log          # context: 3 lines after, 1 before (-C 2: both sides)
grep -m 1 'ready' app.log               # stop after the first match
grep -q 'ready' app.log && echo up      # quiet: only the exit status (0 = found)
```

Across a tree:

```bash
grep -rn 'listen' /etc/nginx                    # recursive, with file name and line number
grep -rl 'TLSv1' /etc --include='*.conf'        # only the names of files WITH a match
grep -L 'server_name' sites/*.conf              # the names of files WITHOUT a match
grep -rh 'listen' /etc/nginx | sort | uniq -c   # -h: no file names, ready for counting
```

### Regular expressions (use -E)

| Pattern | Matches |
|---|---|
| `^` and `$` | start and end of the line: `^#` comment lines, `^$` empty lines |
| `.` and `\.` | any character, and a literal dot |
| `[0-9]`, `[^0-9]`, `[[:space:]]` | one character from a set, not from the set, blank or tab |
| `*`, `+`, `?`, `{2,4}` | the item before it: any number, at least one, optional, 2 to 4 times |
| `(warn\|error)` | either one; brackets also group for `?` and `+` |
| `\<` and `\>`, or `\b` | the edge of a word |

```bash
grep -E '^[[:space:]]*listen +[0-9]+' nginx.conf       # an active directive, not a commented one
grep -E ' (ERROR|FATAL) ' app.log                      # one of two levels, as a field
grep -E '(^|[^0-9.])10\.20\.[0-9]+\.[0-9]+' hosts      # an address, not the tail of 110.20...
```

Quote patterns with single quotes, or the shell eats `*`, `$` and `|` before grep sees them.

### Count and rank: cut, sort, uniq

```bash
cut -d: -f1,7 /etc/passwd                       # fields 1 and 7, delimiter ':'
cut -d' ' -f4 app.log | sort | uniq -c | sort -rn      # how often each value of field 4 occurs
sort -t: -k3,3n /etc/passwd                     # whole lines by field 3 as a number (r: reversed)
sort -u names.txt                               # sorted, duplicates removed
grep ' ERROR ' app.log | head -n 1              # first occurrence       | tail -n 1   last
```

- `uniq` only merges **neighbouring** equal lines: always `sort` first.
- `sort` compares text unless told `-n`: as text, `999` is larger than `31007` and `9` larger than `10`.
- `cut` needs a single-character delimiter and prints lines without it unchanged (`-s` drops them).
  For columns separated by runs of blanks use `awk '{print $2}'`.

### Logs that are still being written

```bash
tail -f app.log                                  # print new lines as they arrive; Ctrl-C ends tail only
tail -n 0 -f app.log | grep --line-buffered ERROR       # only new lines, filtered live
tail -F app.log                                  # keeps following when the file is rotated
less +F app.log                                  # follow in less; Ctrl-C to scroll and search, F to resume
```

Trigger the action while you follow, note the marker (a request id, a timestamp) and collect the
lines with `grep` afterwards: the file keeps them.

## Habits that save points

- Prove a pattern on a sample (`| head`, `grep -c`, compare with `wc -l`) before you trust a count.
- Match the field, not the word: `grep ERROR` also finds "no ERROR today"; `grep payments` also
  finds `payments-worker`.
- Decide whether comments count. `grep -v '^[[:space:]]*#'` first, or anchor the pattern with `^`.
- Never edit or empty a log to make searching easier. Filters read; the source stays as it is.
- An empty result with exit status 1 is an answer too: check `echo $?` before you assume a typo.
