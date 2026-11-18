Write the script in the order it runs, and try each part with the sample before adding the next.

```bash
ssh {{host}}
cd /opt/course/collect-logs-script
vim collect-logs.sh
```

```bash
#!/usr/bin/env bash
# collect-logs.sh <source-dir> <dest-dir>

if [ $# -ne 2 ]; then
  echo "usage: collect-logs.sh <source-dir> <dest-dir>" >&2
  exit 2
fi

src=$1
dst=$2

if [ ! -d "$src" ]; then
  echo "collect-logs: $src: not a directory" >&2
  exit 3
fi

mkdir -p "$dst" || exit 1

n=0
for f in "$src"/*.log; do
  if [ -f "$f" ] && [ -s "$f" ]; then
    cp "$f" "$dst"/ && n=$((n + 1))
  fi
done

echo "collected $n"
[ "$n" -gt 0 ]
```

| Line | Why |
|---|---|
| `[ $# -ne 2 ]` | `$#` is the number of arguments; validate before anything is touched |
| `>&2` | messages for humans go to standard error, so standard output stays clean for the result |
| `exit 2`, `exit 3` | one code per kind of failure: the caller can react without reading text |
| `"$src"/*.log` | the variable is quoted, the pattern is not: bash expands it to whole names, blanks included |
| `[ -f "$f" ] && [ -s "$f" ]` | regular file, and not empty. Skips the directory `rotated.log`, `empty.log`, and the literal `*.log` that the loop gets when nothing matches |
| `cp ... && n=$((n + 1))` | count only what was really copied |
| `[ "$n" -gt 0 ]` | a script exits with the code of its last command: 0 if true, 1 if false |

```bash
chmod +x collect-logs.sh
./collect-logs.sh "sample/web server" "out/first try"; echo "exit code: $?"    # collected 2, 0
ls -l "out/first try"
./collect-logs.sh; echo "exit code: $?"                                        # usage, 2
./collect-logs.sh nowhere out/x; echo "exit code: $?"                          # not a directory, 3
./collect-logs.sh out out/y; echo "exit code: $?"                              # collected 0, 1
```

What goes wrong most often: `for f in $(ls $src/*.log)` splits names at blanks, a missing `chmod +x`
gives "Permission denied", and `echo` of the error message without `>&2` puts it on standard output.

**Takeaway:** a script is an interface: validate the arguments, quote every expansion, print results
to stdout and complaints to stderr, and say how it went with the exit code.
