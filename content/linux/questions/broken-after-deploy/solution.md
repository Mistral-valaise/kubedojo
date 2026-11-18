Three sources, read together: what the service **says** (log), what was **changed** (config against
its backup), and what the service is **allowed** to do (permissions, as its own user).

**1. Reproduce and read the log.**

```bash
ssh {{host}}
cd /opt/course/broken-after-deploy
cat CHANGE-4711.txt
wc -l var/log/orders.log                 # 11: everything after that is new
sudo ./orderctl start                    # fails, and points to the log
tail -n 3 var/log/orders.log
# ERROR startup failed: data_dir /opt/course/broken-after-deploy/var/dta is not a directory
```

**2. Compare the configuration with the one that worked.**

```bash
diff etc/orders.conf.bak etc/orders.conf
# < data_dir = .../var/data          > data_dir = .../var/dta       <- typo
# < max_orders = 100                 > max_orders = 500             <- the planned change
ls var/
vim etc/orders.conf                      # /dta  then  cw data <Esc>  :wq   (or: sed -i 's|/var/dta$|/var/data|')
```

Do not copy the backup back: that would also undo the new limit. Do not create `var/dta` either:
the service would start, with no orders.

**3. Start again: the next error appears.**

```bash
sudo ./orderctl start
tail -n 3 var/log/orders.log
# INFO  loaded 3 orders from .../var/data, max_orders 500
# ERROR startup failed: cannot read token_file .../etc/api.token as user broken-after-deploy: Permission denied
ls -l etc/api.token                      # -rw------- root root
id broken-after-deploy
```

The deployment job wrote the new token as root with mode 600, so only root can read it. The service
does not run as root.

**4. Grant exactly the missing access.**

```bash
sudo chgrp broken-after-deploy etc/api.token
sudo chmod 640 etc/api.token             # owner rw, group r, others nothing
sudo -u broken-after-deploy head -c 4 etc/api.token; echo     # test as the service account
```

`sudo chown broken-after-deploy etc/api.token` (mode 600) works as well. `chmod 644` would
"work" too, and let every user on the host read the secret.

**5. Start and verify.**

```bash
sudo ./orderctl start
curl -s http://127.0.0.1:23180/health | jq .
# {"status": "ok", "orders_loaded": 3, "max_orders": 500}
ss -ltne | grep 23180                    # uid:... is the owner of the socket (none shown: root)
ps -o user:22,pid,cmd -C python3
```

**Takeaway:** log, diff, permissions. Fix one error, start again, read again; test access as the
user the service runs as; and make the smallest change that removes the cause.
