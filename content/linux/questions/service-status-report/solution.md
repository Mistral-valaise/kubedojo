Two building blocks, tried at the prompt first: a test for one port and a loop over a file.

```bash
ssh {{host}}
cd /opt/course/service-status-report
nc -z -w 2 127.0.0.1 21080; echo $?      # 0: something listens
nc -z -w 2 127.0.0.1 21090; echo $?      # 1: connection refused
vim check-services.sh
```

`nc -z` only opens the connection and closes it; `-w 2` gives up after two seconds, so that a host
that does not answer cannot stall the script.

```bash
#!/usr/bin/env bash
# check-services.sh <list-file> <report-file>

if [ $# -ne 2 ]; then
  echo "usage: check-services.sh <list-file> <report-file>" >&2
  exit 2
fi

list=$1
report=$2

if [ ! -f "$list" ] || [ ! -r "$list" ]; then
  echo "check-services: cannot read $list" >&2
  exit 2
fi

is_up() {
  nc -z -w 2 "$1" "$2" 2>/dev/null
}

failed=0
while read -r name host port || [ -n "$name" ]; do
  case $name in
    '' | '#'*) continue ;;
  esac
  if is_up "$host" "$port"; then
    echo "$name UP"
  else
    echo "$name DOWN"
    failed=1
  fi
done < "$list" > "$report"

exit "$failed"
```

| Part | Why |
|---|---|
| `read -r name host port` | one line per round, split at runs of blanks into three variables |
| `\|\| [ -n "$name" ]` | `read` fails on a last line without newline, although it filled the variables |
| `case $name in '' \| '#'*)` | blank lines leave `name` empty; comments start with `#` |
| `is_up` | a function is a command of your own: its exit code is that of its last command, and `if` tests it directly |
| `done < "$list" > "$report"` | the loop reads the list and its whole output replaces the report: no `>>`, nothing doubles on the second run |
| `failed=1` ... `exit "$failed"` | the loop runs in the script's own shell, so the variable survives it |

The checks before the loop matter: with a missing list, the redirection `> "$report"` would still
create an empty report.

```bash
chmod +x check-services.sh
./check-services.sh services.txt report.txt; echo "exit code: $?"; cat report.txt    # 1, three lines
./check-services.sh services.txt report.txt; cat report.txt                          # still three lines
./check-services.sh nowhere.txt report.txt; echo "exit code: $?"                     # 2
```

The trap that costs the exit code: `grep -v '^#' "$list" | while read ...; do failed=1; done`. Every
part of a pipeline runs in a subshell, and `failed` is 0 again after `done`. Feed the loop with a
redirection (`done < file`) or a process substitution (`done < <(command)`).

**Takeaway:** `while read -r ...; done < file` is the loop for lists; keep state in variables of
the main shell and turn it into an exit code at the end.
