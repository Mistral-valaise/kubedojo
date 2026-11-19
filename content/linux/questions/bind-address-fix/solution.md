"Works on the host, connection refused from elsewhere" is almost always the **bind address**.

**1. Reproduce it from both sides.**

```bash
curl -m 5 http://{{host}}:23080/metrics          # on the terminal: connection refused
ssh {{host}}
curl -m 5 http://localhost:23080/metrics         # on the host: exporter_up 1
```

Refused, not a timeout: the host answers, so it is neither DNS nor routing.

**2. Look at the listening sockets.**

```bash
sudo ss -ltnp
# State  Recv-Q Send-Q Local Address:Port  Peer Address:Port Process
# LISTEN 0      5          127.0.0.1:23080      0.0.0.0:*     users:(("python3",pid=...))
# LISTEN 0      5          127.0.0.1:23081      0.0.0.0:*     users:(("python3",pid=...))
# LISTEN 0      128          0.0.0.0:22         0.0.0.0:*     users:(("sshd",pid=...))
```

`-l` listening, `-t` TCP, `-n` numbers instead of names, `-p` the process (needs `sudo` for
processes of other users). The local address decides who may connect: `127.0.0.1` only this
machine, `0.0.0.0` every IPv4 address of the host, as sshd does. `ps -fp <pid>` shows the command
line and with it the configuration file.

**3. Fix the configuration, and only the metrics endpoint.**

```bash
cd /opt/course/bind-address-fix
vim exporter.conf            # listen_address = 0.0.0.0   (admin_address stays 127.0.0.1)
```

In Vim: `/listen_address` jumps to the line, `$` to its end, `ciw` or `cW` replaces the address.
A `:%s/127.0.0.1/0.0.0.0/` would also publish the admin endpoint.

**4. Restart, because the file is only read at start, and look again.**

```bash
sudo ./exporterctl restart
ss -ltn | grep -E ':2308[01]'
# LISTEN 0 5   0.0.0.0:23080 ...
# LISTEN 0 5 127.0.0.1:23081 ...
printf '%s\n' 0.0.0.0:23080 127.0.0.1:23081 > listeners.txt
```

**5. Test like the client.**

```bash
exit                                              # back on the terminal
curl http://{{host}}:23080/metrics                # exporter_up 1
curl -m 5 http://{{host}}:23081/-/reload          # still refused: as intended
```

**Takeaway:** `ss -ltnp` answers "who listens where". Refused from outside but fine on localhost
means a loopback bind; change it in the service's configuration, restart, and verify from the
client's side.
