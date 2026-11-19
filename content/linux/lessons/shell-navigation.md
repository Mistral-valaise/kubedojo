# Shell & Navigation

**8% of the skills assessment.** Know where you are, find what you need, and ask the system instead
of guessing: every other domain builds on these few commands.

## The 20% that solves 80%

### Where am I, what is here

```bash
pwd                         # the directory you are in
cd /var/log                 # absolute path: starts at /
cd nginx                    # relative path: starts where you are
cd ..                       # one level up        cd -   back to the previous directory
cd                          # home directory      ~/x    means /home/<you>/x
ls -la                      # long format, hidden entries (names starting with a dot) included
ls -lt | head               # newest first        ls -lS   largest first      ls -ld dir   the directory itself
ls -lh /var/log/*.log       # human-readable sizes; the shell expands the pattern, not ls
```

Reading `ls -l`: the first character is the type (`-` file, `d` directory, `l` symbolic link), then
the permissions, the link count, owner, group, size, modification time and the name.

Tab completes names and shows the alternatives; it also proves that a path exists before you run
anything. `Ctrl-r` searches the command history, `!$` is the last argument of the previous command.

### find: search by property, not by luck

```bash
find /opt/app -name '*.log'                       # by name; quote the pattern
find /opt/app -iname 'readme*'                    # ignore case
find /opt/app -type f -size +100k -mtime +30      # regular files, > 100 KiB, modified > 30 days ago
find /opt/app -type d -empty                      # empty directories
find /opt/app -maxdepth 1 -type f                 # do not descend
find /opt/app -newer /opt/app/build.stamp         # changed after that file
find / -name 'id_*' 2>/dev/null                   # hide "Permission denied"
find /opt/app -name '*.tmp' -exec ls -l {} +      # run a command on the results
find /opt/app -name '*.log' -printf '%s %TY-%Tm-%Td %p\n' | sort -n      # size, date, path
```

- All tests must hold (AND). `-o` is OR and needs brackets: `\( -name '*.log' -o -name '*.txt' \)`.
- Numbers: `+30` more than, `-30` less than, `30` exactly. Sizes need a unit: `k`, `M`, `G`; without
  one, `-size` counts 512-byte blocks.
- `-type f` keeps directories and symbolic links with a matching name out of the result.
- The output is in no particular order: add `| sort` when an order matters.

### What does this name run?

bash looks a name up in this order: **alias, function, builtin, file in `PATH`**. The first hit wins.

```bash
type ls                     # ls is aliased to `ls --color=auto'
type -a kill                # every definition, in order: the builtin, then /usr/bin/kill
type -t cd                  # one word: alias | keyword | function | builtin | file
type -P python3             # the file a PATH search finds
echo "$PATH" | tr ':' '\n'  # the directories, in search order
command -v jq >/dev/null || echo "jq is not installed"       # the test for scripts
```

- `which` is a separate program: it searches `PATH` only and knows nothing about aliases, functions
  and builtins. Trust `type`.
- A file that is not executable is not a command, however early its directory is in `PATH`.
- "It works in my shell but not in the script / in cron": compare `type -a <name>` and `echo $PATH`
  in both places. A script has no aliases, and cron has a short `PATH`.
- A command in the current directory needs a path: `./deploy.sh`. `.` is not in `PATH`.
- `\ls` or `command ls` skips an alias for one call; `hash -r` makes bash forget remembered locations.

### Ask the system: --help, man, help

```bash
tar --help | less           # the short option list of a program
man tar                     # the manual page; inside: /word search, n next, N previous, q quit
man 5 crontab               # section 5 (file formats) instead of 1 (commands)
man -k compress             # search the titles of all pages (same as apropos)
man -l ./tool.1             # open a manual file that is not installed
help cd                     # builtins (cd, type, read, test ...) are documented by bash itself
```

Manual pages have the same sections everywhere, which makes them fast to scan: SYNOPSIS (the forms
of the call; `[x]` optional, `...` repeatable), OPTIONS, EXIT STATUS (what scripts test), ENVIRONMENT
(variables that change the behaviour), FILES, EXAMPLES. Jump with `/^EXIT` instead of scrolling.

An option that takes a value is searched fastest as `/^ *-s` or `/--sort`. After the first hit, `n`.

```bash
grep --help | grep -i -- 'count'        # find the option for a job; -- ends grep's own options
VAR=value command                       # set an environment variable for this one call only
command; echo $?                        # exit status of the last command: 0 = success
```

## Habits that save points

- Read the task for **absolute paths** and write results with them: `> /opt/course/x/result.txt`
  works from every directory, `> result.txt` only from the right one.
- Quote patterns for `find` and `grep` (`'*.log'`); leave them unquoted where the shell should
  expand them (`ls *.log`). Quote variables and names with spaces: `"$f"`.
- Look before you act: run `find ...` alone, read the list, then add `-delete` or `-exec`.
- Unknown tool? `--help` first (ten seconds), `man` and `/SECTION` second. Guessing options costs
  more time than reading, and a plausible wrong output is worse than an error.
- `cat` the result file before you move on: it is what gets graded.
