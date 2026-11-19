`type` is the shell's own answer to "what would you run for this name?". `which` is a separate
program that only searches `PATH`: it cannot see aliases, functions or builtins.

```bash
ssh {{host}}
source /opt/course/command-lookup/team-env.sh
cd /opt/course/command-lookup

type rollout stage kill deployctl        # the long answer, to read
type -a kill                             # every definition: the builtin first, then /usr/bin/kill

for n in rollout stage kill deployctl; do echo "$n $(type -t "$n")"; done > kinds.txt
type -P deployctl > winner.txt           # -P: search PATH only, print the path
type -aP deployctl > candidates.txt      # -a: all matches, in lookup order
```

| Step | What it shows |
|---|---|
| `type -t` | one word per name: `alias`, `function`, `builtin`, `file` (or `keyword`). bash tries them in that order, so the builtin `kill` shadows `/usr/bin/kill` |
| `type -P` | the file that wins the `PATH` search: `legacy/bin/deployctl`, because `team-env.sh` lists `legacy/bin` before `tools/bin` |
| `type -aP` | all files that could run, first match first. `scratch/bin/deployctl` is missing although its directory is first in `PATH`: it has no execute permission (`ls -l`), so bash walks past it |

`./legacy/bin/deployctl` and `./tools/bin/deployctl` print their versions, which confirms the story:
the rollout used 1.4. The repair would be to change the order in `PATH`; here only the evidence
was asked for.

`source` changes the shell you are in. To keep your login shell as it was, start a second one with
`bash` before sourcing and leave it with `exit` when you are done.

In a script, `command -v deployctl` gives the same answer as `type -P` for a file and is the
portable way to test whether a tool exists. After changing `PATH` in a running shell, `hash -r`
makes bash forget the locations it remembered.

**Takeaway:** ask the shell with `type -a <name>` before you trust a command name; use `which` only
when you know the name is a file.
