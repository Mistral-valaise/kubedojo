# Processes & Services

**10% of the skills assessment.** Find the one process you mean, tell it what to do with the right
signal, and make sure a service is not only started but working.

## The 20% that solves 80%

### Find the process

```bash
ps -ef                                  # everything, with parent pid and command line
ps -eo pid,ppid,user:20,stat,etime,args # choose the columns; user:20 avoids "candida+"
ps -o pid,stat,args -p 4711             # one pid          ps -o pid,args -C nginx   # by name
pgrep -a exporter                       # pids and command lines of processes with that name
pstree -p                               # who started whom
ss -ltnp                                # listening TCP ports and the process behind each
ss -ltnp | grep :8080                   # who has port 8080?
lsof -p 4711                            # files and sockets a process holds open
ls -l /proc/4711/cwd /proc/4711/fd; tr '\0' ' ' < /proc/4711/cmdline
```

STAT column: `R` running, `S` sleeping (normal), `T` stopped, `Z` zombie (ended, parent has not
collected it: signal the parent, not the zombie).

A name is often not unique. Identify a process by what is: its port, its arguments, its pidfile.
`ss -p` and `lsof` show only processes you may inspect: on a real host use `sudo` to see all of them;
in this sandbox root may not look into other users' processes, so run them without `sudo` for your own.

### Signals

```bash
kill 4711            # SIGTERM (15): please stop. The default, and the first thing to try
kill -HUP 4711       # SIGHUP (1): terminal gone; daemons by convention re-read their configuration
kill -INT 4711       # SIGINT (2): what Ctrl-C sends
kill -KILL 4711      # SIGKILL (9): cannot be caught or ignored. Last resort
kill -STOP 4711; kill -CONT 4711      # freeze and continue (Ctrl-Z sends the catchable SIGTSTP)
kill -l              # all names and numbers
pkill -x name        # by exact name: signals EVERY match. killall name: the same
```

- `kill` only sends. Check the result: `ps -p 4711`, `ss -ltn`.
- After SIGKILL the process has cleaned up nothing: stale locks, pidfiles and temporary files are yours.
- What a signal means beyond the defaults is up to the program: read its documentation before `-HUP`.
  A program without a handler **dies** from SIGHUP.
- Signalling processes of other users needs `sudo`.

### Jobs: foreground, background, and surviving the logout

```bash
long-command &            # start in the background           jobs -l   # the jobs of THIS shell
Ctrl-Z                    # stop the foreground job            bg        # continue it in the background
fg %1                     # bring job 1 back                   kill %1   # signal job 1

nohup long-command >> /var/tmp/job.log 2>&1 < /dev/null &   # survives logout and a lost connection
setsid long-command >> /var/tmp/job.log 2>&1 < /dev/null &  # own session, no terminal at all
disown %1                 # already running: take it off the shell's list, so it gets no SIGHUP
```

- `2>&1` must come **after** `>> file`: it means "where standard output goes right now".
- Without a redirection `nohup` writes to `nohup.out` in the current directory.
- `jobs`, `fg` and `bg` know only the jobs of the shell you type in. For anything else: `ps` and `kill`.

### Services with `service` (this sandbox)

The sandbox host has **no systemd**. Services are shell scripts in `/etc/init.d`, driven by `service`
(it lives in `/usr/sbin`, which is not in a normal user's `PATH`: call it with `sudo`):

```bash
sudo service --status-all            # [ + ] running, [ - ] stopped
sudo service nginx start             # also stop, restart, reload, status
sudo service nginx status; echo $?   # 0 = running, 3 = not running
cat /etc/init.d/nginx                # the script: where is the pidfile, the log, the configuration?
cat /etc/default/nginx               # local settings the script reads
```

When a service does not start: 1. `status`, 2. its log (`/var/log/<name>...`), 3. the path, port or
permission the message names, 4. start again, 5. ask it: `curl -s localhost:<port>`, `ss -ltnp`.
"Address already in use" = another process listens on the port.

### On a systemd machine

Not available in this sandbox, but what you will meet on almost every real host and exam node:

```bash
systemctl status nginx               # state, pid, the last log lines
sudo systemctl start|stop|restart|reload nginx
sudo systemctl enable --now nginx    # start now and at every boot      (disable --now: the opposite)
systemctl cat nginx                  # the unit file and its drop-ins
systemctl --failed                   # units that did not start
journalctl -u nginx -e               # the log of one unit, jump to the end     -f: follow
journalctl -u nginx --since '10 min ago'
sudo systemctl daemon-reload         # after editing a unit file
```

The method is the same: status, log, fix, restart, verify.

## Habits that save points

- Record before you kill: `ps -o pid,user:20,lstart,args -p <pid>`. A dead process has no command line.
- SIGTERM, look, and only then SIGKILL. `kill -9` as a first step loses data.
- `pkill` and `killall` take every process with that name: use the pid when siblings must live.
- "Running" is not "working": test the port or the output, not only the status.
- Do not move a service to another port to dodge a conflict: its clients still use the old one.
- A changed configuration file changes nothing until the process reloads or restarts.
