**1. The collector**

```bash
ssh {{host}}
cd /opt/course/keep-job-running
nohup bin/keepjob-collect >> collector.log 2>&1 < /dev/null &
jobs                          # [1]+ Running  nohup bin/keepjob-collect ...
pgrep -a keepjob-collect      # exactly one
cat collector.log             # the start message and the warning
```

Piece by piece:

| Piece | What it does |
|---|---|
| `nohup` | the command ignores SIGHUP, the signal a job gets when its terminal goes away |
| `>> collector.log` | standard output is appended to the file |
| `2>&1` | standard error goes where standard output goes *now*: it must come **after** the file |
| `< /dev/null` | no input from the terminal; otherwise `ssh` may hang at logout waiting for the job |
| `&` | run in the background, the prompt comes back |

Without your own redirection `nohup` writes to `nohup.out` in the current directory.

Other ways to the same end:

```bash
setsid bin/keepjob-collect >> collector.log 2>&1 < /dev/null &   # own session, no terminal at all
bin/keepjob-collect >> collector.log 2>&1 < /dev/null &          # already started without nohup?
disown                                                           # ... then take it off the shell's job list
```

`disown` only makes *your shell* forget the job, so that it no longer passes SIGHUP on to it; the
process stays attached to the terminal until the shell exits. That is why the check accepts it only
after you have logged out. If you started the collector twice while trying, stop the extra one:
`kill <pid>`.

**2. The suspended import**

```bash
ps -o pid,stat,args -C keepjob-import
#  PID STAT COMMAND
# 6021 T    /usr/bin/python3 /opt/course/keep-job-running/bin/keepjob-import
kill -CONT 6021               # the pid from your output
ps -o pid,stat,args -C keepjob-import      # S: sleeping, which is what a waiting process does
```

`T` means stopped. `jobs`, `fg` and `bg` only know the jobs of the shell you are typing in; this
process belongs to somebody else's shell, so the signal has to be sent by hand. In your own shell
the same thing is: Ctrl-Z (stop), `bg` (continue in the background), `fg` (bring it back).

**Takeaway:** `nohup cmd >> log 2>&1 < /dev/null &` for anything that must survive your session, and
`kill -CONT` to wake a process that shows `T` in `ps`.
