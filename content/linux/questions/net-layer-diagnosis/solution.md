A URL can fail at three layers, and each has its own tool. Work from the bottom up and stop at the
first layer that fails; everything above it cannot work anyway.

```bash
ssh {{host}}
cd /opt/course/net-layer-diagnosis
cat endpoints.txt
```

**1. DNS: does the name resolve?**

```bash
getent hosts inventory.net-layer-diagnosis.invalid; echo "exit $?"   # no output, exit 2
dig +short inventory.net-layer-diagnosis.invalid                     # empty answer
getent hosts {{host}}                                                # an address: fine
```

`getent hosts` asks the way applications do (`/etc/hosts` first, then DNS); `dig` asks the DNS
server only. `inventory` stops here: `dns`.

**2. TCP: does the port accept a connection?**

```bash
nc -zv {{host}} 23013        # Connection refused -> tcp
nc -zv {{host}} 23014        # succeeded
ss -ltn                      # on the host itself: which ports have a listener at all
```

"Connection refused" is an answer: the host is reachable, but no process listens on 23013.
`search` stops here: `tcp`.

**3. HTTP: what does the application answer?**

```bash
for p in 23011 23014 23015; do
  curl -s -o /dev/null -m 5 -w "$p %{http_code}\n" http://{{host}}:$p/health
done
# 23011 200
# 23014 503
# 23015 404
curl -i http://{{host}}:23014/health     # headers and body: "database unavailable"
```

Both `checkout` and `profile` have an open port, so a port test calls them healthy. The status code
does not: 503 is a server that cannot do its work, 404 means the status page polls a path this
service does not have.

```bash
printf '%s\n' 'billing ok' 'inventory dns' 'search tcp' 'checkout http-503' 'profile http-404' > report.txt
```

`curl` alone can tell the layers apart, which is handy in a loop: exit code 6 is "could not resolve
host", 7 is "failed to connect", and with exit code 0 the status is in `%{http_code}`.

**Takeaway:** name, port, status. Ask the three questions in that order, and never call a service
healthy because its port is open.
