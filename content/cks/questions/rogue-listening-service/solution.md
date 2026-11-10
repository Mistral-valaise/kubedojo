```bash
ssh {{host}}
ssh {{worker}}
sudo -i
ss -ltnp | grep 9977
# LISTEN 0 1 0.0.0.0:9977 0.0.0.0:* users:(("telemetry-relay",pid=2314,fd=3))
ls -l /proc/2314/exe
# /proc/2314/exe -> /usr/local/sbin/telemetry-relay
systemctl status 2314 | head -3
# ● telemetry-relay.service - Telemetry relay
#      Loaded: loaded (/etc/systemd/system/telemetry-relay.service; enabled; ...)
```

Stop it, keep it from starting again, remove it:

```bash
systemctl disable --now telemetry-relay
rm /usr/local/sbin/telemetry-relay
rm /etc/systemd/system/telemetry-relay.service && systemctl daemon-reload     # tidy up the unit too
ss -ltnp | grep 9977                                                          # nothing
exit; exit
echo /usr/local/sbin/telemetry-relay > /opt/course/listener/binary
```

Only `kill` would not have helped: the unit has `Restart=always`, so systemd restarts the process
two seconds later.

**Host hardening checklist:** list listening ports (`ss -ltnp`), running services
(`systemctl list-units --type=service --state=running`), installed packages you do not need
(`apt list --installed`), and users with a login shell (`grep -v nologin /etc/passwd`).
