# Bash Automation

**8% of the skills assessment.** A script is a command of your own: it takes arguments, does its work
for any file name, and reports through its exit code. Most broken scripts break in the same five places.

## The 20% that solves 80%

### The frame of every script

```bash
#!/usr/bin/env bash
# backup.sh <source-dir> <dest-dir>
set -euo pipefail                    # stop on errors, on unset variables, on a failing pipe stage

if [ $# -ne 2 ]; then                # $# = number of arguments
  echo "usage: backup.sh <source-dir> <dest-dir>" >&2      # complaints go to stderr
  exit 2                             # and get their own exit code
fi
src=$1
dst=$2
```

```bash
chmod +x backup.sh && ./backup.sh "my docs" /tmp/out; echo "exit code: $?"
bash -n backup.sh                    # syntax check without running; bash -x backup.sh traces every command
```

`$1 $2 ...` are the arguments, `"$@"` all of them (each one kept whole), `$0` the script itself.

### Quote every expansion

```bash
file="release notes.txt"
rm $file          # WRONG: two arguments, "release" and "notes.txt"
rm "$file"        # one argument
cp "$src"/*.log "$dst"/              # quote the variable, leave the pattern outside the quotes
now=$(date +%F)                      # no blanks around = in an assignment
```

Unquoted, a value is split at blanks and its `*` is expanded. `"$var"` and `"$(command)"` are the rule;
single quotes `'$var'` switch expansion off completely.

### Exit status, && and ||

Every command returns 0 (success) or 1 to 255 (failure). `if` tests exactly that.

```bash
grep -q '^root:' /etc/passwd && echo found           # && runs the next command on success
mkdir -p "$dst" || { echo "cannot create $dst" >&2; exit 1; }     # || on failure
cmd; rc=$?                           # save $? at once: the next command overwrites it
cmd | tee log; rc=${PIPESTATUS[0]}   # after a pipe, $? is the code of the LAST command
exit "$rc"                           # without exit, a script returns the code of its last command
```

Printing "failed" does not make a script fail. Only the exit code does.

### Tests

```bash
[ -f "$p" ]      # regular file      [ -d "$p" ]  directory       [ -e "$p" ]  exists
[ -s "$p" ]      # not empty         [ -r "$p" ]  readable        [ -x "$p" ]  executable
[ -z "$s" ]      # empty string      [ -n "$s" ]  not empty       [ "$a" = "$b" ]  same text
[ "$n" -eq 3 ]   # numbers: -eq -ne -lt -le -gt -ge. Never < or >: inside [ ] those are redirections
[ -f "$p" ] && [ -s "$p" ]           # and; ! negates: [ ! -d "$p" ]
```

Blanks around `[`, `]` and the operator are required. `[ -d $dir ]` with an empty `$dir` is **true**.

```bash
case $1 in
  start|stop) echo "doing $1" ;;
  -h|--help)  echo "usage: ..." ;;
  *)          echo "unknown: $1" >&2; exit 2 ;;
esac
```

### Loops

```bash
for f in "$dir"/*.log; do            # over file names: a pattern, never $(ls ...)
  [ -f "$f" ] || continue            # no match leaves the literal *.log: test it
  gzip -k "$f"
done

while read -r name host port || [ -n "$name" ]; do   # over the lines of a file, split into fields
  case $name in ''|'#'*) continue ;; esac            # skip blank lines and comments
  echo "$name -> $host:$port"
done < services.txt                  # the || part keeps a last line without newline
```

- `for x in $(cat file)` splits at blanks, not at lines. Use `while IFS= read -r line`.
- `cmd | while read ...` runs the loop in a subshell: variables set inside are gone afterwards. Feed the
  loop with `done < file` or `done < <(cmd)`.
- A command inside the loop that reads stdin (`ssh`) eats the rest of the list: give it `< /dev/null`.

### Functions

```bash
is_up() { nc -z -w 2 "$1" "$2" 2>/dev/null; }     # arguments are $1, $2; the result is the exit code
if is_up 127.0.0.1 8080; then echo UP; else echo DOWN; failed=1; fi
```

## Habits that save points

- Test the unhappy paths too: no arguments, a name with a blank, a missing file, an empty directory.
- Make it safe to run twice: `mkdir -p`, `rm -f`, `>` instead of `>>`, check before you create.
- Keep state in a variable (`failed=1`) and turn it into the exit code at the end: `exit "$failed"`.
- Under `set -e`, a command that may fail needs `|| true` or an `if`; `grep` without a match exits 1.
