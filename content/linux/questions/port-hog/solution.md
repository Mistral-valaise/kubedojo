Three processes share a name, so the name does not identify the culprit: the port does.

```bash
ssh {{host}}
pgrep -a port-hog-worker        # three of them; nothing here says which port each one uses
ss -ltnp | grep :22110
# LISTEN 0 8 127.0.0.1:22110 0.0.0.0:* users:(("port-hog-worker",pid=4711,fd=3))
```

`ss` options: `-l` listening sockets, `-t` TCP, `-n` numbers instead of service names, `-p` the
owning process. The process column is only filled for processes whose details you may read. These
workers are your own, so run `ss` **without** `sudo`; on a normal machine `sudo ss -ltnp` shows the
processes of all users (in this sandbox root is not allowed to look into other users' processes).

**Record, then act** (use the pid from your output):

```bash
ps -o pid,user:20,args -p 4711
#  PID USER       COMMAND
# 4711 candidate  /usr/bin/python3 /opt/course/port-hog/bin/port-hog-worker --queue billing
cd /opt/course/port-hog
{
  echo "pid=4711"
  echo "user=$(ps -o user:20= -p 4711 | tr -d ' ')"
  echo "command=$(ps -o args= -p 4711)"
} > culprit.txt
```

`ps -ef` prints `candida+` because its user column is eight characters wide; `user:20` asks for a
wider one. The `=` after a column name removes the header.

**Stop it gracefully**

```bash
kill 4711                       # SIGTERM: "please finish"
ps -p 4711                      # gone?
ss -ltn | grep :22110           # no output: the port is free
cat state/worker-4711.exit      # clean shutdown after SIGTERM
```

Not `kill -9`: SIGKILL cannot be caught, the worker would have no chance to finish. Not
`pkill port-hog-worker`: that ends the two healthy workers too.

Other ways to get from a port to a pid: `lsof -i TCP:22110 -s TCP:LISTEN`, `fuser 22110/tcp`.

**Takeaway:** identify a process by what makes it unique (here its port), write down pid, user and
command line while it lives, and send SIGTERM to that one pid.
