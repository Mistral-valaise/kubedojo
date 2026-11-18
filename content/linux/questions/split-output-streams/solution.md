Every program starts with two output streams: **standard output** (file descriptor 1) for the result
and **standard error** (file descriptor 2) for diagnostics. A terminal shows both, which is why they
look like one. Each can be redirected on its own.

```bash
ssh {{host}}
cd /opt/course/split-output-streams
bin/inventory-export > export.csv 2> warnings.log; echo $? > exit-code.txt
cat export.csv warnings.log exit-code.txt
```

| Part | Meaning |
|---|---|
| `> export.csv` | standard output goes to the file (short for `1>`) |
| `2> warnings.log` | standard error goes to its own file |
| `echo $?` | the exit code of the command that ran **last**: here the tool, because nothing ran in between |

The tool exits with `3`: it worked, but skipped three records. A job that never looks at `$?` would
report success.

Traps:

- `> export.csv 2>&1` and `&> export.csv` send **both** streams into one file. `2>&1` means "descriptor
  2 goes where descriptor 1 goes now".
- `bin/inventory-export | tee export.csv; echo $?` prints the exit code of `tee`, which is 0. In a
  pipeline, the codes of all commands are in the array `${PIPESTATUS[@]}`:
  `bin/inventory-export 2> warnings.log | tee export.csv; echo "${PIPESTATUS[0]}"`.
- Any command between the tool and `echo $?` (even `ls`) overwrites `$?`. Save it at once: `rc=$?`.

**Takeaway:** `>` is for the result, `2>` for the complaints, `$?` for the verdict, and `$?` must be
read immediately.
