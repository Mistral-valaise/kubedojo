Start it, read what it says, fix one cause, repeat.

```bash
ssh {{host}}
sudo service inventoryd start
# Starting inventoryd: failed. See /var/log/inventoryd.log
sudo service inventoryd status      # inventoryd is not running
cat /var/log/inventoryd.log
# ... init: starting inventoryd with --config /etc/inventory/inventory.conf
# inventoryd: cannot read configuration /etc/inventory/inventory.conf: No such file or directory
```

**Cause 1: a wrong path**

```bash
ls /etc/inventoryd                  # inventoryd.conf: the file exists, under another name
grep -n CONFIG /etc/init.d/inventoryd /etc/default/inventoryd
```

The init script has the right default and then reads `/etc/default/inventoryd`, which overrides it
with the old path. That file is the place for local settings; the init script itself stays as it is.

```bash
sudo vim /etc/default/inventoryd    # CONFIG=/etc/inventoryd/inventoryd.conf
sudo service inventoryd start       # fails again, for a new reason
cat /var/log/inventoryd.log
# inventoryd: cannot listen on 127.0.0.1:22210: Address already in use
```

**Cause 2: the port is taken**

```bash
ss -ltnp | grep :22210
# LISTEN ... 127.0.0.1:22210 ... users:(("inventoryd-old",pid=7310,fd=3))
ps -o pid,user:20,lstart,args -p 7310
curl http://127.0.0.1:22210/healthz     # inventoryd 1.9 (test instance)
kill 7310                               # the pid from your output; SIGTERM is enough
ss -ltn | grep :22210                   # nothing: free
```

Run `ss` without `sudo` here: the test instance is your own process (see `linux/port-hog`). Changing
`port=` in the configuration would make the service start, but its clients use 22210 and would keep
talking to the old instance.

**Start and verify**

```bash
sudo service inventoryd start       # Starting inventoryd: ok (pid ...)
sudo service inventoryd status      # inventoryd is running (pid ...)
curl http://127.0.0.1:22210/healthz # inventoryd 2.4 ok
curl -s http://127.0.0.1:22210/items | jq length
```

On a systemd machine the same investigation is `systemctl status inventoryd`,
`journalctl -u inventoryd -e`, `systemctl cat inventoryd` for the unit and its drop-ins, and
`systemctl restart inventoryd`. The method does not change.

**Takeaway:** status, log, fix, start again; and after "it runs", ask the service on its real port
whether it works.
